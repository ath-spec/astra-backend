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

	// IDBI Atlas gateway integration. IDBIBaseURL defaults to the sandbox; the
	// gateway allow-lists the caller's egress IP, so no key/token is sent.
	// Each feature is gated by its own flag — all default OFF, meaning the app
	// behaves exactly as before (mock/seeded data). Turning a flag on makes
	// that feature read its idbi_* mirror instead.
	IDBIBaseURL            string
	IDBIAccountsEnabled    bool // feature 1: real account balances (365/394)
	IDBISpendEnabled       bool // feature 2: real transactions in spend analytics (393)
	IDBILoansEnabled       bool // feature 3: My Loans (391/402/538)
	IDBIKYCEnabled         bool // feature 6: CKYC verification (415)
	IDBICreditScoreEnabled bool // feature 8: credit score (mock until 408 works)
	IDBIRMRiskEnabled      bool // feature 5: RM credit-risk view (442/402)
	IDBIAAEnabled          bool // feature 4: Account Aggregator consent flow (590/591/592/497/498/595)
	IDBIHRMSLoginEnabled   bool // feature 7: verify staff EIN against HRMS (508) before issuing an RM login code

	// AA-flow tuning (only read when IDBIAAEnabled). Load() fills the blanks
	// from IDBI_AA_* — there are no literals buried in the service layer.
	IDBIAARedirectMode string // IDBI_AA_REDIRECT_MODE: "stub" | "live"
	IDBIAACallbackURL  string // IDBI_AA_CALLBACK_URL: redirectUrl handed to 592 in "live" mode
	IDBIAAProductID    string // IDBI_AA_PRODUCT_ID
	IDBIAAVUASuffix    string // IDBI_AA_VUA_SUFFIX: appended to the mobile to form the VUA

	// CKYC caller identity (only read when IDBIKYCEnabled). All from IDBI_CKYC_*.
	IDBICKYCParentCompany string // IDBI_CKYC_PARENT_COMPANY
	IDBICKYCAPIToken      string // IDBI_CKYC_API_TOKEN
	IDBICKYCBranchCode    string // IDBI_CKYC_BRANCH_CODE
	IDBICKYCSourceSystem  string // IDBI_CKYC_SOURCE_SYSTEM
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

	cfg.IDBIBaseURL = envOr("IDBI_BASE_URL", "https://sandboxpocgatewayprod.idbi.bank.in")
	cfg.IDBIAccountsEnabled = os.Getenv("IDBI_ACCOUNTS_ENABLED") == "true"
	cfg.IDBISpendEnabled = os.Getenv("IDBI_SPEND_ENABLED") == "true"
	cfg.IDBILoansEnabled = os.Getenv("IDBI_LOANS_ENABLED") == "true"
	cfg.IDBIKYCEnabled = os.Getenv("IDBI_KYC_ENABLED") == "true"
	cfg.IDBICreditScoreEnabled = os.Getenv("IDBI_CREDIT_SCORE_ENABLED") == "true"
	cfg.IDBIRMRiskEnabled = os.Getenv("IDBI_RM_RISK_ENABLED") == "true"
	cfg.IDBIAAEnabled = os.Getenv("IDBI_AA_ENABLED") == "true"
	cfg.IDBIHRMSLoginEnabled = os.Getenv("IDBI_HRMS_LOGIN_ENABLED") == "true"
	cfg.IDBIAARedirectMode = envOr("IDBI_AA_REDIRECT_MODE", "stub")
	cfg.IDBIAACallbackURL = os.Getenv("IDBI_AA_CALLBACK_URL") // only used in "live" mode
	cfg.IDBIAAProductID = envOr("IDBI_AA_PRODUCT_ID", "TEST")
	cfg.IDBIAAVUASuffix = envOr("IDBI_AA_VUA_SUFFIX", "@onemoney")
	cfg.IDBICKYCParentCompany = os.Getenv("IDBI_CKYC_PARENT_COMPANY")
	cfg.IDBICKYCAPIToken = os.Getenv("IDBI_CKYC_API_TOKEN")
	cfg.IDBICKYCBranchCode = os.Getenv("IDBI_CKYC_BRANCH_CODE")
	cfg.IDBICKYCSourceSystem = envOr("IDBI_CKYC_SOURCE_SYSTEM", "Finacle")

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

// envOr returns the environment variable value, or fallback when it is unset
// or empty. Used so tunable defaults live here in the config layer, in one
// visible place, rather than as literals scattered through the services.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
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
