package config

import (
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/yourusername/astra-backend/internal/crypto"
)

type Config struct {
	Port              string
	AppAuthToken      string
	GroqAPIKey        string
	DatabaseURL       string
	JWTSecret         string
	MasterInternalKey string
	SarvamAPIKey      string
}

func Load() *Config {
	_ = godotenv.Load()

	masterKey := os.Getenv("MASTER_INTERNAL_KEY")

	cfg := &Config{
		Port:              os.Getenv("PORT"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		MasterInternalKey: masterKey,
		
		// These might be encrypted, we will check below
		AppAuthToken:      os.Getenv("APP_AUTH_TOKEN"),
		GroqAPIKey:        os.Getenv("GROQ_API_KEY"),
		JWTSecret:         os.Getenv("JWT_SECRET"),
		SarvamAPIKey:      os.Getenv("SARVAM_API_KEY"),
	}

	// If MASTER_INTERNAL_KEY is provided and is 32 characters, we attempt decryption
	if len(masterKey) == 32 {
		log.Println("🔐 MASTER_INTERNAL_KEY detected. Decrypting internal secrets...")
		cfg.AppAuthToken = decryptOrFatal(cfg.AppAuthToken, masterKey, "APP_AUTH_TOKEN")
		cfg.GroqAPIKey = decryptOrFatal(cfg.GroqAPIKey, masterKey, "GROQ_API_KEY")
		cfg.JWTSecret = decryptOrFatal(cfg.JWTSecret, masterKey, "JWT_SECRET")
		cfg.SarvamAPIKey = decryptOrFatal(cfg.SarvamAPIKey, masterKey, "SARVAM_API_KEY")
	} else if masterKey != "" {
		log.Fatalf("FATAL: MASTER_INTERNAL_KEY is set but is %d characters long (must be 32).", len(masterKey))
	} else {
		log.Println("⚠️ MASTER_INTERNAL_KEY not set. Falling back to plain text secrets (Not recommended for production).")
	}

	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	if cfg.AppAuthToken == "" {
		log.Println("WARNING: APP_AUTH_TOKEN is not set. API is vulnerable.")
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
