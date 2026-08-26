package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"

	"github.com/yourusername/astra-backend/internal/config"
	"github.com/yourusername/astra-backend/internal/database"
	"github.com/yourusername/astra-backend/internal/handler"
	authmw "github.com/yourusername/astra-backend/internal/middleware"
	analyticsprovider "github.com/yourusername/astra-backend/internal/provider/analytics"
	catalogprovider "github.com/yourusername/astra-backend/internal/provider/catalog"
	fdprovider "github.com/yourusername/astra-backend/internal/provider/fd"
	paymentsprovider "github.com/yourusername/astra-backend/internal/provider/payments"
	stocksprovider "github.com/yourusername/astra-backend/internal/provider/stocks"
	"github.com/yourusername/astra-backend/internal/repository"
	"github.com/yourusername/astra-backend/internal/service"
	analyticsservice "github.com/yourusername/astra-backend/internal/service/analytics"
)

func main() {
	ctx := context.Background()

	// 1. Load Configuration
	cfg := config.Load()

	// 2. Apply schema migrations, then initialize the application DB pool.
	if err := database.RunMigrations(cfg.DatabaseURL); err != nil {
		log.Fatalf("Could not apply database migrations: %v", err)
	}

	db, err := database.NewDatabase(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Could not initialize database: %v", err)
	}
	defer db.Close()

	// 3. Initialize Repositories
	userRepo := repository.NewPostgresUserRepository(db)
	chatRepo := repository.NewPostgresChatRepository(db)

	// 4. Initialize Services
	aiService := service.NewGroqAIService(cfg.GroqAPIKey, cfg.SarvamAPIKey, chatRepo)
	authService := service.NewAuthService(cfg.JWTSecret)

	stocksService := service.NewStocksService(stocksprovider.NewMockProvider(db.Pool))
	catalogService := service.NewCatalogService(catalogprovider.NewMockProvider(db.Pool))
	fdService := service.NewFDService(fdprovider.NewMockProvider(db.Pool, userRepo))
	paymentsService := service.NewPaymentsService(paymentsprovider.NewMockProvider(db.Pool, userRepo))
	spendAnalyticsService := analyticsservice.NewService(analyticsprovider.NewMockSource(db.Pool), userRepo)

	// 5. Initialize Handlers
	chatHandler := handler.NewChatHandler(aiService, userRepo, chatRepo)
	authHandler := handler.NewAuthHandler(authService, userRepo)
	stocksHandler := handler.NewStocksHandler(stocksService)
	catalogHandler := handler.NewCatalogHandler(catalogService)
	fdHandler := handler.NewFDHandler(fdService)
	paymentsHandler := handler.NewPaymentsHandler(paymentsService)
	analyticsHandler := handler.NewAnalyticsHandler(spendAnalyticsService)
	aaHandler := handler.NewAAHandler()
	kycHandler := handler.NewKYCHandler()
	mfHandler := handler.NewMFHandler()

	// 6. Setup Router
	r := chi.NewRouter()

	// Base Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	// VerboseLogger dumps full request bodies/headers (including
	// Authorization) to logs — it must never run by default in a deployed
	// environment. Opt in locally only, with DEBUG_VERBOSE_LOG=true.
	if os.Getenv("DEBUG_VERBOSE_LOG") == "true" {
		r.Use(authmw.VerboseLogger)
	}
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Rate Limiting (50 requests per minute per IP)
	r.Use(httprate.LimitByIP(50, 1*time.Minute))

	// Custom 404 Handler
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "Route not found", "code": 404}`))
	})

	// CORS Setup - Protects against web abuse by restricting to trusted frontend domains
	allowedOriginsStr := os.Getenv("FRONTEND_URL")
	var allowedOrigins []string

	if allowedOriginsStr == "" {
		// Secure default lockdown based on your exact provided domain
		allowedOrigins = []string{"https://astraaaaaa.netlify.app", "http://localhost:*", "http://127.0.0.1:*"}
	} else {
		// Split by comma in case multiple frontend URLs are passed in the environment variable
		for _, origin := range strings.Split(allowedOriginsStr, ",") {
			allowedOrigins = append(allowedOrigins, strings.TrimSpace(origin))
		}
	}

	// Always allow the known production frontend and local development
	allowedOrigins = append(allowedOrigins, "https://astraaaaaa.netlify.app", "http://localhost:*", "http://127.0.0.1:*")

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: allowedOrigins,
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type", "X-Astra-Auth"},
		MaxAge:         300,
	}))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Astra Backend is running with JWT Authentication!"))
	})

	// Unprotected Routes (OTP Flow)
	r.Group(func(r chi.Router) {
		r.Post("/api/auth/otp/send", authHandler.SendOTP)
		r.Post("/api/auth/otp/verify", authHandler.VerifyOTP)
	})

	// Protected Routes (Requires JWT Bearer Token)
	r.Group(func(r chi.Router) {
		r.Use(authmw.RequireAuth(authService))
		r.Post("/api/chat", chatHandler.HandleChat)
		r.Get("/api/chat/history", chatHandler.GetHistory)
		r.Post("/api/tts", chatHandler.HandleTTS) // Moved to JWT-protected route

		// v1 financial domain APIs (see the IDBI sandbox spec doc).
		r.Mount("/api/v1/stocks", stocksHandler.Routes())
		r.Mount("/api/v1/catalog", catalogHandler.Routes())
		r.Mount("/api/v1/fd", fdHandler.Routes())
		r.Mount("/api/v1/payments", paymentsHandler.Routes())
		r.Mount("/api/v1/analytics/spend", analyticsHandler.Routes())

		// Scaffolded only: routed, but return 501 until a provider is picked.
		r.Mount("/api/v1/aa", aaHandler.Routes())
		r.Mount("/api/v1/kyc", kycHandler.Routes())
		r.Mount("/api/v1/mf", mfHandler.Routes())
	})

	// 7. Start Server with Graceful Shutdown
	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		log.Printf("Starting server on port %s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server startup failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited properly")
}
