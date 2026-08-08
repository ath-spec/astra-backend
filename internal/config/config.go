package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port         string
	AppAuthToken string
	GroqAPIKey   string
	DatabaseURL  string
	JWTSecret    string
}

func Load() *Config {
	_ = godotenv.Load()

	cfg := &Config{
		Port:         os.Getenv("PORT"),
		AppAuthToken: os.Getenv("APP_AUTH_TOKEN"),
		GroqAPIKey:   os.Getenv("GROQ_API_KEY"),
		DatabaseURL:  os.Getenv("DATABASE_URL"),
		JWTSecret:    os.Getenv("JWT_SECRET"),
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

	if cfg.DatabaseURL == "" {
		log.Println("WARNING: DATABASE_URL is not set. Database features will fail.")
	}

	if cfg.JWTSecret == "" {
		log.Println("WARNING: JWT_SECRET is not set. Authentication will fail.")
	}

	return cfg
}
