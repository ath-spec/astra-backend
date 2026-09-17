package main

import (
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables from .env file
	if err := godotenv.Load("../.env"); err != nil {
		log.Println("Warning: .env file not found, using system environment variables")
	}

	// Get configuration from environment
	port := getEnv("SERVER_PORT", "8080")
	host := getEnv("SERVER_HOST", "localhost")

	// Validate required environment variables
	if os.Getenv("GEMINI_API_KEY") == "" {
		log.Fatal("Error: GEMINI_API_KEY environment variable is required")
	}

	// Set up routes
	http.HandleFunc("/health", corsMiddleware(healthHandler))
	http.HandleFunc("/upload", corsMiddleware(uploadHandler))
	http.HandleFunc("/extract", corsMiddleware(extractHandler))
	http.HandleFunc("/fast-extract", corsMiddleware(fastExtractHandler))

	// Log startup
	log.Println("Bank Statement RAG Backend Server")
	log.Println("=================================")
	log.Println("Configuration:")
	log.Printf("  Host: %s", host)
	log.Printf("  Port: %s", port)
	log.Printf("  Gemini API Key: %s", maskAPIKey(os.Getenv("GEMINI_API_KEY")))
	log.Println("")
	log.Println("Endpoints:")
	log.Println("  GET  /health  - Health check")
	log.Println("  POST /upload  - Upload PDF file")
	log.Println("  GET  /extract?file=<path> - Extract transactions from PDF (RAG)")
	log.Println("  GET  /fast-extract?file=<path> - Fast extract transactions (OCR)")
	log.Println("")
	log.Printf("Server starting on %s:%s...", host, port)

	// Start server
	addr := host + ":" + port
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
