package config

import (
	"log"
	"os"
	"strings"

	"github.com/yourusername/astra-backend/internal/crypto"
)

type Config struct {
	Port              string
	GroqAPIKey        string
	DatabaseURL       string
	JWTSecret         string
	RMJWTSecret       string
	RMOTPDevCode      string
	MasterInternalKey string
	SarvamAPIKey      string

	// RedisURL is optional. When set (redis:// or rediss://), the shared cache
	// connector is available for cross-replica state — rate limiting, shared
	// response cache, agent session state. Unset today: single API instance,
	// Postgres fingerprint cache is sufficient.
	RedisURL string

	// BudgetMLBaseURL / BudgetMLToken point at the budget-bloc ML service
	// (Hugging Face Space). Only two endpoints are called — POST /ml/diagnosis
	// and POST /suggest/categories. When unreachable the budget service falls
	// back to local heuristics, so the token is optional.
	BudgetMLBaseURL string
	BudgetMLToken   string
}

func Load() *Config {
	masterKey := os.Getenv("MASTER_INTERNAL_KEY")

	cfg := &Config{
		Port:              os.Getenv("PORT"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		RedisURL:          os.Getenv("REDIS_URL"),
		MasterInternalKey: masterKey,

		// These might be encrypted, we will check below
		GroqAPIKey:   os.Getenv("GROQ_API_KEY"),
		JWTSecret:    os.Getenv("JWT_SECRET"),
		RMJWTSecret:  os.Getenv("RM_JWT_SECRET"),
		SarvamAPIKey: os.Getenv("SARVAM_API_KEY"),

		BudgetMLBaseURL: os.Getenv("BUDGET_ML_BASE_URL"),
		BudgetMLToken:   os.Getenv("BUDGET_ML_TOKEN"),
	}

	if cfg.BudgetMLBaseURL == "" {
		cfg.BudgetMLBaseURL = "https://zeyro87-budget-bloc.hf.space/api/v1"
	}

	// If MASTER_INTERNAL_KEY is provided and is 32 characters, we attempt decryption
	if len(masterKey) == 32 {
		log.Println("🔐 MASTER_INTERNAL_KEY detected. Decrypting internal secrets...")
		cfg.GroqAPIKey = decryptOrFatal(cfg.GroqAPIKey, masterKey, "GROQ_API_KEY")
		cfg.JWTSecret = decryptOrFatal(cfg.JWTSecret, masterKey, "JWT_SECRET")
		cfg.SarvamAPIKey = decryptOrFatal(cfg.SarvamAPIKey, masterKey, "SARVAM_API_KEY")
		if cfg.RMJWTSecret != "" {
			cfg.RMJWTSecret = decryptOrFatal(cfg.RMJWTSecret, masterKey, "RM_JWT_SECRET")
		}
		if cfg.BudgetMLToken != "" {
			cfg.BudgetMLToken = decryptOrFatal(cfg.BudgetMLToken, masterKey, "BUDGET_ML_TOKEN")
		}
	} else if masterKey != "" {
		log.Fatalf("FATAL: MASTER_INTERNAL_KEY is set but is %d characters long (must be 32).", len(masterKey))
	} else {
		log.Println("⚠️ MASTER_INTERNAL_KEY not set. Falling back to plain text secrets (Not recommended for production).")
	}

	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	if cfg.GroqAPIKey == "" {
		log.Println("WARNING: GROQ_API_KEY is not set. Chat calls will fail.")
	}

	if cfg.SarvamAPIKey == "" {
		log.Println("WARNING: SARVAM_API_KEY is not set. TTS calls will fail.")
	}

	if cfg.DatabaseURL == "" {
		log.Println("WARNING: DATABASE_URL is not set. Database features will fail.")
	}

	if cfg.JWTSecret == "" {
		log.Println("WARNING: JWT_SECRET is not set. Authentication will fail.")
	}

	// RM/Admin console auth uses its own signing key so a leaked user token
	// can never be replayed against staff endpoints and vice versa. Falling
	// back to JWT_SECRET keeps local dev working, but the two must be
	// distinct in any deployed environment.
	if cfg.RMJWTSecret == "" {
		log.Println("WARNING: RM_JWT_SECRET is not set. Falling back to JWT_SECRET for the RM/Admin console.")
		cfg.RMJWTSecret = cfg.JWTSecret
	}

	// RM_OTP_DEV_CODE, when set, makes the RM/Admin console accept that
	// fixed code for every OTP verification — for testing without SMS
	// delivery wired in. Never set this in production.
	cfg.RMOTPDevCode = os.Getenv("RM_OTP_DEV_CODE")
	if cfg.RMOTPDevCode != "" {
		log.Printf("WARNING: RM_OTP_DEV_CODE is set — RM/Admin OTP login will accept the fixed code %q.", cfg.RMOTPDevCode)
	}

	return cfg
}

func decryptOrFatal(ciphertext, key, varName string) string {
	if ciphertext == "" {
		return ""
	}

	// If it doesn't look like base64, warn the user (maybe they forgot to encrypt it in the env)
	if !strings.HasSuffix(ciphertext, "=") && !strings.ContainsAny(ciphertext, "+/") && len(ciphertext) < 20 {
		log.Printf("WARNING: %s does not look like a base64 encrypted string. Decryption may fail.", varName)
	}

	plaintext, err := crypto.Decrypt(ciphertext, key)
	if err != nil {
		log.Fatalf("FATAL: Failed to decrypt %s: %v", varName, err)
	}
	return plaintext
}
