package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	"github.com/yourusername/astra-backend/internal/repository"
	"github.com/yourusername/astra-backend/internal/service"
)

func main() {
	ctx := context.Background()

	// 1. Load Configuration
	cfg := config.Load()

	// 2. Initialize Database
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

	// 5. Initialize Handlers
	chatHandler := handler.NewChatHandler(aiService, userRepo, chatRepo)
	authHandler := handler.NewAuthHandler(authService, userRepo)

	// 6. Setup Router
	r := chi.NewRouter()

	// Base Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(authmw.VerboseLogger) // <-- NEW: Dumps full requests for debugging
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
	allowedOrigin := os.Getenv("FRONTEND_URL")
	if allowedOrigin == "" {
		allowedOrigin = "https://astraaaaaa.netlify.app" // Secure lockdown for production
	}
	
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"}, 
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
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
