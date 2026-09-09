package main

import (
	"context"
	"errors"
	"log/slog"
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

	"github.com/yourusername/astra-backend/internal/ai/agents"
	"github.com/yourusername/astra-backend/internal/config"
	"github.com/yourusername/astra-backend/internal/database"
	"github.com/yourusername/astra-backend/internal/handler"
	authmw "github.com/yourusername/astra-backend/internal/middleware"
	analyticsprovider "github.com/yourusername/astra-backend/internal/provider/analytics"
	budgetprovider "github.com/yourusername/astra-backend/internal/provider/budget"
	catalogprovider "github.com/yourusername/astra-backend/internal/provider/catalog"
	fdprovider "github.com/yourusername/astra-backend/internal/provider/fd"
	goalsprovider "github.com/yourusername/astra-backend/internal/provider/goals"
	idbiprovider "github.com/yourusername/astra-backend/internal/provider/idbi"
	"github.com/yourusername/astra-backend/internal/provider/llm"
	mfprovider "github.com/yourusername/astra-backend/internal/provider/mf"
	paymentsprovider "github.com/yourusername/astra-backend/internal/provider/payments"
	"github.com/yourusername/astra-backend/internal/provider/speech"
	stocksprovider "github.com/yourusername/astra-backend/internal/provider/stocks"
	watchlistprovider "github.com/yourusername/astra-backend/internal/provider/watchlist"
	"github.com/yourusername/astra-backend/internal/repository"
	"github.com/yourusername/astra-backend/internal/rmseed"
	"github.com/yourusername/astra-backend/internal/service"
	advisortipsservice "github.com/yourusername/astra-backend/internal/service/advisortips"
	analyticsservice "github.com/yourusername/astra-backend/internal/service/analytics"
	budgetservice "github.com/yourusername/astra-backend/internal/service/budget"
	creditscoreservice "github.com/yourusername/astra-backend/internal/service/creditscore"
	idbiaaservice "github.com/yourusername/astra-backend/internal/service/idbiaa"
	idbiaccountsservice "github.com/yourusername/astra-backend/internal/service/idbiaccounts"
	idbihrmsservice "github.com/yourusername/astra-backend/internal/service/idbihrms"
	idbikycservice "github.com/yourusername/astra-backend/internal/service/idbikyc"
	idbileadsservice "github.com/yourusername/astra-backend/internal/service/idbileads"
	idbiloansservice "github.com/yourusername/astra-backend/internal/service/idbiloans"
	rmcreditriskservice "github.com/yourusername/astra-backend/internal/service/rmcreditrisk"
	statementsyncservice "github.com/yourusername/astra-backend/internal/service/statementsync"
)

func main() {
	ctx := context.Background()

	// 1. Load Configuration
	cfg := config.Load()

	// Configure the global structured (JSON) logger. Every slog.Info/Warn/Error
	// call — including the StructuredLogger middleware — writes to stdout as a
	// JSON object. CloudWatch / Loki can ingest this without any format config.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		// ReplaceAttr converts the built-in "time" key from an ISO-8601 string
		// to Unix epoch seconds, matching the project-wide apitime wire format.
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Int64(slog.TimeKey, a.Value.Time().Unix())
			}
			return a
		},
	})))

	// 2. Apply schema migrations, then initialize the application DB pool.
	if err := database.RunMigrations(cfg.DatabaseURL); err != nil {
		slog.Error("database migration failed", "error", err)
		os.Exit(1)
	}

	db, err := database.NewDatabase(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database init failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Optional: seed the RM/Admin starter desk (1 admin + 2 RMs) and
	// backfill unassigned users on boot. Idempotent; opt in with
	// RM_SEED_ON_BOOT=true. Non-fatal — a seeding failure must not stop the
	// API from serving.
	if os.Getenv("RM_SEED_ON_BOOT") == "true" {
		if res, serr := rmseed.Run(ctx, db.Pool, rmseed.ConfigFromEnv()); serr != nil {
			slog.Warn("RM_SEED_ON_BOOT: seeding failed (continuing)", "error", serr)
		} else {
			slog.Info("RM_SEED_ON_BOOT: staff seeded", "users_assigned", res.UsersAssigned)
		}
	}

	// 3. Initialize Repositories
	userRepo := repository.NewPostgresUserRepository(db)
	chatRepo := repository.NewPostgresChatRepository(db)
	chatMemoryRepo := repository.NewPostgresChatMemoryRepository(db.Pool)

	// 4. Initialize Services

	// Provider-agnostic LLM + speech seams. Groq / Sarvam are wired today;
	// Bedrock / Polly+Transcribe are provisioned and switch on via
	// LLM_PROVIDER / SPEECH_PROVIDER after the AWS review — no call-site change.
	var groqModels []string
	if cfg.LLMGroqModels != "" {
		for _, m := range strings.Split(cfg.LLMGroqModels, ",") {
			if m = strings.TrimSpace(m); m != "" {
				groqModels = append(groqModels, m)
			}
		}
	}
	llmProvider := llm.New(llm.Config{
		Provider: cfg.LLMProvider,
		Groq:     llm.GroqConfig{APIKey: cfg.GroqAPIKey, Models: groqModels},
		Bedrock: llm.BedrockConfig{
			Region: cfg.BedrockRegion, ModelID: cfg.BedrockModelID,
			AgentID: cfg.BedrockAgentID, AgentAliasID: cfg.BedrockAgentAliasID,
		},
	})
	speechProvider := speech.New(speech.Config{
		Provider: cfg.SpeechProvider,
		Sarvam:   speech.SarvamConfig{APIKey: cfg.SarvamAPIKey},
		AWS: speech.AWSConfig{
			Region: cfg.SpeechAWSRegion, PollyVoice: cfg.SpeechPollyVoice, STTLang: cfg.SpeechSTTLang,
		},
	})

	// Routing catalog for every LLM-backed surface (chat assistants, RM/Admin
	// copilots, RM narrator, memory extractor). Per-agent Bedrock Agent ids
	// come from BEDROCK_AGENT_<KEY>_ID; until set, each runs on its model list
	// exactly as today.
	agentCatalog := agents.New(agents.Overrides{
		BedrockAgentIDs: map[agents.Key]string{
			agents.KeyAppChat:      cfg.BedrockChatAgentIDs["app_chat"],
			agents.KeyAppQuickChat: cfg.BedrockChatAgentIDs["app_quickchat"],
			agents.KeyRMCopilot:    cfg.BedrockChatAgentIDs["rm_copilot"],
			agents.KeyAdminCopilot: cfg.BedrockChatAgentIDs["admin_copilot"],
			agents.KeyRMNarrator:   cfg.BedrockChatAgentIDs["rm_narrator"],
			agents.KeyMemory:       cfg.BedrockChatAgentIDs["memory"],
		},
	})
	slog.Info("AI providers", "llm", llmProvider.Name(), "speech", speechProvider.Name())

	aiService := service.NewGroqAIService(llmProvider, speechProvider, agentCatalog, chatRepo)
	memoryService := service.NewMemoryService(chatMemoryRepo, llmProvider, agentCatalog)

	var advisorTipsSvc *advisortipsservice.Service
	if cfg.AITipsEnabled {
		advisorTipsSvc = advisortipsservice.New(llmProvider, advisortipsservice.Config{
			AgentIDs: map[advisortipsservice.Topic]string{
				advisortipsservice.TopicAllocation:  cfg.BedrockTipAgentIDs["allocation"],
				advisortipsservice.TopicDiscipline:  cfg.BedrockTipAgentIDs["discipline"],
				advisortipsservice.TopicPerformance: cfg.BedrockTipAgentIDs["performance"],
				advisortipsservice.TopicFundProfile: cfg.BedrockTipAgentIDs["fund_profile"],
			},
		}, slog.Default())
	}
	authService := service.NewAuthService(cfg.JWTSecret)

	stocksProvider := stocksprovider.NewMockProvider(db.Pool)
	fdProvider := fdprovider.NewMockProvider(db.Pool, userRepo)
	mfProvider := mfprovider.NewMockProvider(db.Pool)

	watchlistProvider := watchlistprovider.NewPostgresProvider(db.Pool)

	stocksService := service.NewStocksService(stocksProvider)
	portfolioAnalysisService := service.NewPortfolioAnalysisService(mfProvider, stocksProvider, fdProvider, db.Pool)
	catalogService := service.NewCatalogService(catalogprovider.NewMockProvider(db.Pool), mfProvider, watchlistProvider, portfolioAnalysisService)
	watchlistService := service.NewWatchlistService(watchlistProvider)
	fdService := service.NewFDService(fdProvider)
	paymentsService := service.NewPaymentsService(paymentsprovider.NewMockProvider(db.Pool, userRepo))
	// Spend transaction source. Default: the seeded MockSource. When
	// IDBI_SPEND_ENABLED is on, read spend_transactions as-is (PgSource) and
	// let statementsync fill it from IDBI 393 / AA 595 instead.
	var spendSource analyticsprovider.TransactionSource = analyticsprovider.NewMockSource(db.Pool)
	if cfg.IDBISpendEnabled {
		spendSource = analyticsprovider.NewPgSource(db.Pool)
		slog.Info("IDBI feature enabled: real transactions in spend analytics (spend_transactions)")
	}
	spendAnalyticsService := analyticsservice.NewService(spendSource, analyticsprovider.NewPgInvestmentSource(db.Pool), userRepo)
	goalsProvider := goalsprovider.NewPostgresProvider(db.Pool)
	goalsService := service.NewGoalsService(goalsProvider)
	mfService := service.NewMFService(mfProvider)
	dashboardService := service.NewDashboardService(stocksProvider, mfProvider, fdProvider, userRepo, db.Pool)

	// Budget feature: setup-wizard sessions -> finalized monthly budgets ->
	// active dashboard. Spend history is read from the same seeded
	// transaction source the analytics engine uses; ML suggestions come from
	// budget-bloc (POST /ml/diagnosis, POST /suggest/categories) with local
	// heuristic fallbacks.
	budgetRepo := repository.NewBudgetRepository(db.Pool)
	budgetMLClient := budgetprovider.NewClient(cfg.BudgetMLBaseURL, cfg.BudgetMLToken)
	budgetService := budgetservice.NewService(budgetRepo, spendSource, budgetMLClient)

	// IDBI Atlas integration. The client always exists (cheap); each feature
	// service is only constructed + routed when its flag is on. Flag off =>
	// the app reads its existing mock/seeded data, unchanged.
	idbiClient := idbiprovider.NewClient(idbiprovider.Config{BaseURL: cfg.IDBIBaseURL})
	idbiRepo := repository.NewIDBIRepository(db.Pool)
	var idbiAccountsSvc *idbiaccountsservice.Service
	var idbiSpendSvc *statementsyncservice.Service
	if cfg.IDBIAccountsEnabled {
		idbiAccountsSvc = idbiaccountsservice.New(idbiClient, idbiRepo, idbiaccountsservice.Config{}, slog.Default())
		slog.Info("IDBI feature enabled: account balances (idbi_accounts)")
	}
	if cfg.IDBISpendEnabled {
		idbiSpendSvc = statementsyncservice.New(idbiClient, idbiRepo, statementsyncservice.Config{}, slog.Default())
	}
	var idbiLoansSvc *idbiloansservice.Service
	if cfg.IDBILoansEnabled {
		idbiLoansSvc = idbiloansservice.New(idbiClient, idbiRepo, idbiloansservice.Config{}, slog.Default())
		slog.Info("IDBI feature enabled: My Loans (idbi_loans)")
	}
	var idbiKYCSvc *idbikycservice.Service
	if cfg.IDBIKYCEnabled {
		idbiKYCSvc = idbikycservice.New(idbiClient, db.Pool, idbikycservice.Config{
			ParentCompany: cfg.IDBICKYCParentCompany,
			APIToken:      cfg.IDBICKYCAPIToken,
			BranchCode:    cfg.IDBICKYCBranchCode,
			SourceSystem:  cfg.IDBICKYCSourceSystem,
			AppFormNo:     cfg.IDBICKYCAppFormNo,
		})
		slog.Info("IDBI feature enabled: CKYC verification (415)")
	}
	var creditScoreSvc *creditscoreservice.Service
	if cfg.IDBICreditScoreEnabled {
		// 408 is dead in the sandbox — use the mock source. Swap to
		// creditscoreservice.IDBISource{Client: idbiClient} when ACC fixes creds.
		creditScoreSvc = creditscoreservice.New(creditscoreservice.MockSource{}, 24*time.Hour)
		slog.Info("IDBI feature enabled: credit score (mock source — 408 unavailable in sandbox)")
	}
	var idbiLeadsSvc *idbileadsservice.Service
	if cfg.IDBILeadsEnabled {
		idbiLeadsSvc = idbileadsservice.New(idbiClient, idbileadsservice.Config{
			DefaultSolID: cfg.IDBILeadSolID,
			LeadChannel:  cfg.IDBILeadChannel,
			LeadSource:   cfg.IDBILeadSource,
		})
		slog.Info("IDBI feature enabled: product-interest leads (362 createLead)")
	}
	idbiHandler := handler.NewIDBIHandler(idbiAccountsSvc, idbiSpendSvc, idbiLoansSvc, creditScoreSvc, idbiLeadsSvc)

	// Feature 4: Account Aggregator consent flow. Off => aa_handler keeps its
	// original stub behaviour (fake CONSENT-xxxx, no state).
	var idbiAASvc *idbiaaservice.Service
	if cfg.IDBIAAEnabled {
		idbiAARepo := repository.NewIDBIAARepository(db.Pool)
		idbiAASvc = idbiaaservice.New(idbiClient, idbiAARepo, idbiRepo, idbiaaservice.Config{
			RedirectMode:    cfg.IDBIAARedirectMode,
			CallbackURL:     cfg.IDBIAACallbackURL,
			ProductID:       cfg.IDBIAAProductID,
			VUASuffix:       cfg.IDBIAAVUASuffix,
			StatementSource: cfg.IDBIAAStatementSrc,
		}, slog.Default())
		slog.Info("IDBI feature enabled: Account Aggregator consent flow (idbi_aa_consents)", "redirect_mode", cfg.IDBIAARedirectMode)
	}

	var idbiRMHandler *handler.IDBIRMHandler
	{
		var rmRiskSvc *rmcreditriskservice.Service
		if cfg.IDBIRMRiskEnabled {
			rmRiskSvc = rmcreditriskservice.New(idbiClient, idbiRepo, slog.Default())
			slog.Info("IDBI feature enabled: RM credit-risk view (idbi_credit_exposure)")
		}
		idbiRMHandler = handler.NewIDBIRMHandler(rmRiskSvc)
	}

	// RM/Admin console: separate identity, separate auth (RM_JWT_SECRET),
	// separate schema. Composes the user-domain providers read-only.
	rmUserRepo := repository.NewPostgresRMUserRepository(db.Pool)
	assignmentRepo := repository.NewPostgresAssignmentRepository(db.Pool)
	rmInteractionRepo := repository.NewPostgresRMInteractionRepository(db.Pool)
	rmChatRepo := repository.NewPostgresRMChatRepository(db.Pool)
	userRepo.SetAssigner(assignmentRepo) // auto-assign new signups to an RM
	rmAuthService := service.NewRMAuthService(cfg.RMJWTSecret, cfg.RMOTPDevCode, rmUserRepo)
	if cfg.IDBIHRMSLoginEnabled {
		// Feature 7: withhold a login code for an EIN HRMS does not recognise
		// as an active employee. Additive — the local OTP path is unchanged.
		rmAuthService.UseHRMSVerifier(idbihrmsservice.New(idbiClient, ""))
		slog.Info("IDBI feature enabled: HRMS active-employee check on RM login (508)")
	}
	rmService := service.NewRMService(dashboardService, portfolioAnalysisService, stocksProvider, mfProvider, fdProvider, goalsProvider, userRepo, assignmentRepo, rmUserRepo, rmInteractionRepo, llmProvider, agentCatalog, db.Pool)
	rmAdminService := service.NewRMAdminService(rmUserRepo, assignmentRepo)
	rmChatService := service.NewRMChatService(llmProvider, speechProvider, agentCatalog, rmChatRepo, rmService, rmAdminService)

	// 5. Initialize Handlers
	chatHandler := handler.NewChatHandler(
		aiService, userRepo, chatRepo, memoryService,
		dashboardService, portfolioAnalysisService, goalsProvider,
		stocksProvider, mfProvider, fdProvider, watchlistService, spendAnalyticsService,
		db.Pool,
	)
	authHandler := handler.NewAuthHandler(authService, userRepo)
	stocksHandler := handler.NewStocksHandler(stocksService)
	catalogHandler := handler.NewCatalogHandler(catalogService)
	fdHandler := handler.NewFDHandler(fdService)
	paymentsHandler := handler.NewPaymentsHandler(paymentsService)
	analyticsHandler := handler.NewAnalyticsHandler(spendAnalyticsService)
	budgetHandler := handler.NewBudgetHandler(budgetService)
	goalsHandler := handler.NewGoalsHandler(goalsService)
	aaHandler := handler.NewAAHandler(db.Pool)
	if idbiAASvc != nil {
		aaHandler.WithIDBI(idbiAASvc)
	}
	if idbiAccountsSvc != nil {
		aaHandler.WithIDBIAccounts(idbiAccountsSvc)
	}
	kycHandler := handler.NewKYCHandler(idbiKYCSvc)
	mfHandler := handler.NewMFHandler(mfService)
	dashboardHandler := handler.NewDashboardHandler(dashboardService)
	portfolioAnalysisHandler := handler.NewPortfolioAnalysisHandler(portfolioAnalysisService)
	if advisorTipsSvc != nil {
		portfolioAnalysisHandler.WithTips(advisorTipsSvc)
	}
	watchlistHandler := handler.NewWatchlistHandler(watchlistService)
	rmAuthHandler := handler.NewRMAuthHandler(rmAuthService)
	rmHandler := handler.NewRMHandler(rmService)
	rmAdminHandler := handler.NewRMAdminHandler(rmAdminService)
	rmChatHandler := handler.NewRMChatHandler(rmChatService)

	// 6. Setup Router
	r := chi.NewRouter()

	// Base Middleware — execution order matches r.Use() registration order.
	r.Use(middleware.RequestID)     // 1. generate X-Request-Id
	r.Use(middleware.RealIP)        // 2. resolve real client IP
	r.Use(authmw.WithRequestLogger) // 3. inject per-request slog.Logger into ctx
	r.Use(authmw.Interceptor)       // 4. set security headers, echo X-Request-Id in response, log 5xx errors
	r.Use(authmw.StructuredLogger)  // 5. emit one JSON summary line per request
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
		r.Post("/api/auth/refresh", authHandler.Refresh)
		r.Post("/api/auth/logout", authHandler.Logout)
		r.Post("/api/auth/reset", authHandler.ResetUser)

		// Inbound AA notifications (497/498): called by IDBI's gateway, no
		// user JWT. Mounted on a dedicated top-level path (not under the
		// protected /api/v1/aa subtree). Only routed when IDBI_AA_ENABLED.
		if wh := aaHandler.WebhookRoutes(); wh != nil {
			r.Mount("/webhooks/idbi-aa", wh)
		}
	})

	// Protected Routes (Requires JWT Bearer Token)
	r.Group(func(r chi.Router) {
		r.Use(authmw.RequireAuth(authService))
		r.Get("/api/auth/me", authHandler.Me)
		r.Patch("/api/auth/me", authHandler.UpdateMe)
		r.Post("/api/chat", chatHandler.HandleChat)
		r.Get("/api/chat/history", chatHandler.GetHistory)
		r.Get("/api/chat/memory", chatHandler.GetMemory)
		r.Post("/api/chat/memory", chatHandler.AddMemory)
		r.Delete("/api/chat/memory/{id}", chatHandler.DeleteMemory)
		r.Post("/api/tts", chatHandler.HandleTTS) // Moved to JWT-protected route

		// v1 financial domain APIs (see the IDBI sandbox spec doc).
		r.Mount("/api/v1/stocks", stocksHandler.Routes())
		r.Mount("/api/v1/catalog", catalogHandler.Routes())
		r.Mount("/api/v1/fd", fdHandler.Routes())
		r.Mount("/api/v1/payments", paymentsHandler.Routes())
		r.Mount("/api/v1/analytics/spend", analyticsHandler.Routes())
		r.Mount("/api/v1/analytics/budgets", budgetHandler.Routes())
		r.Mount("/api/v1/goals", goalsHandler.Routes())
		r.Mount("/api/v1/dashboard", dashboardHandler.Routes())
		r.Mount("/api/v1/portfolio-analysis", portfolioAnalysisHandler.Routes())
		r.Mount("/api/v1/watchlist", watchlistHandler.Routes())
		// mf: holdings/purchase/redeem/transactions are a real mock provider;
		// only GET /mf/cas (importing an external CAS) stays 501.
		r.Mount("/api/v1/mf", mfHandler.Routes())

		// Scaffolded only: routed, but return 501 until a provider is picked.
		r.Mount("/api/v1/aa", aaHandler.Routes())
		r.Mount("/api/v1/kyc", kycHandler.Routes())

		// IDBI mirror APIs — mounted only when at least one feature flag is on.
		if idbiHandler.Enabled() {
			r.Mount("/api/v1/idbi", idbiHandler.Routes())
		}
	})

	// RM/Admin console API. Entirely separate from the user app above:
	// its own auth (email+password → RM_JWT_SECRET), its own middleware,
	// its own tables. A user JWT is never valid here.
	r.Route("/api/rm", func(r chi.Router) {
		// Unprotected staff auth (employee code / email -> OTP -> tokens).
		r.Post("/auth/otp/send", rmAuthHandler.SendOTP)
		r.Post("/auth/otp/verify", rmAuthHandler.VerifyOTP)
		r.Post("/auth/refresh", rmAuthHandler.Refresh)
		r.Post("/auth/logout", rmAuthHandler.Logout)

		r.Group(func(r chi.Router) {
			r.Use(authmw.RequireRMAuth(rmAuthService))
			r.Get("/auth/me", rmAuthHandler.Me)
			r.Patch("/auth/me", rmAuthHandler.UpdateMe)
			rmHandler.Register(r)     // /clients, /dashboard/summary, ...
			rmChatHandler.Register(r) // /chat, /chat/history, /chat/tts, /chat/stt
			idbiRMHandler.Register(r) // /idbi/clients/{userID}/credit-risk (feature-flagged)

			r.Route("/admin", func(r chi.Router) {
				r.Use(authmw.RequireAdmin)
				rmAdminHandler.Register(r)
			})
		})
	})

	// Optional month-rollover scheduler: opt in with BUDGET_ROLLOVER_SCHEDULER=true.
	// Drafts next month's budget for every user with an active budget, once on
	// boot and daily thereafter (in-process ticker). Non-fatal.
	if os.Getenv("BUDGET_ROLLOVER_SCHEDULER") == "true" {
		go func() {
			defer func() { _ = recover() }()
			t := time.NewTicker(24 * time.Hour)
			defer t.Stop()
			budgetService.RunRollover(context.Background())
			for range t.C {
				budgetService.RunRollover(context.Background())
			}
		}()
		slog.Info("BUDGET_ROLLOVER_SCHEDULER: daily budget rollover enabled")
	}

	// 7. Start Server with Graceful Shutdown
	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		slog.Info("server starting", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server startup failed", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("server shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("forced shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("server exited cleanly")
}
