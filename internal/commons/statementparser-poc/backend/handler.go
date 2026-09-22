package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Transaction represents a bank transaction
type Transaction struct {
	Date            string  `json:"date"`
	Description     string  `json:"description"`
	Amount          float64 `json:"amount"`
	Balance         float64 `json:"balance"`
	TransactionType string  `json:"transaction_type"`
}

// APIResponse represents the API response structure
type APIResponse struct {
	Success      bool          `json:"success"`
	Message      string        `json:"message"`
	Transactions []Transaction `json:"transactions,omitempty"`
	Error        string        `json:"error,omitempty"`
}

// uploadHandler handles PDF file uploads
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("📤 Upload request received from %s", r.RemoteAddr)

	if r.Method != http.MethodPost {
		log.Printf("❌ Invalid method: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form with configurable max file size
	maxFileSizeMB := getEnvInt("MAX_FILE_SIZE_MB", 10)
	err := r.ParseMultipartForm(int64(maxFileSizeMB) << 20)
	if err != nil {
		respondWithError(w, "Failed to parse multipart form", err)
		return
	}

	// Get the file from form data
	file, handler, err := r.FormFile("pdf")
	if err != nil {
		log.Printf("❌ Failed to get file from form: %v", err)
		respondWithError(w, "Failed to get file from form", err)
		return
	}
	defer file.Close()

	log.Printf("📄 Received file: %s (size: %d bytes)", handler.Filename, handler.Size)

	// Create tmp directory if it doesn't exist
	tmpDir := getEnv("TEMP_DIR", "tmp")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		respondWithError(w, "Failed to create tmp directory", err)
		return
	}

	// Generate unique filename
	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("statement_%d_%s", timestamp, handler.Filename)
	filepath := filepath.Join(tmpDir, filename)

	// Create the file
	dst, err := os.Create(filepath)
	if err != nil {
		respondWithError(w, "Failed to create file", err)
		return
	}
	defer dst.Close()

	// Copy uploaded file to destination
	_, err = io.Copy(dst, file)
	if err != nil {
		respondWithError(w, "Failed to save file", err)
		return
	}

	// Return success response with file path
	log.Printf("✅ File uploaded successfully: %s", filename)
	response := APIResponse{
		Success: true,
		Message: fmt.Sprintf("File uploaded successfully: %s", filename),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// loadEnvVars loads environment variables from a .env file
func loadEnvVars(envPath string) ([]string, error) {
	var envVars []string

	file, err := os.Open(envPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE format
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
		}
	}

	return envVars, scanner.Err()
}

// extractHandler handles transaction extraction from PDFs
func extractHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("🔍 Extract request received from %s", r.RemoteAddr)

	if r.Method != http.MethodGet {
		log.Printf("❌ Invalid method: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get file path from query parameter
	filePath := r.URL.Query().Get("file")
	if filePath == "" {
		log.Printf("❌ Missing 'file' query parameter")
		respondWithError(w, "Missing 'file' query parameter", nil)
		return
	}

	log.Printf("📄 Processing file: %s", filePath)

	// Convert relative path to absolute path and normalize for Python execution
	if !filepath.IsAbs(filePath) {
		// Convert Windows backslashes to forward slashes for consistency
		filePath = strings.ReplaceAll(filePath, "\\", "/")

		// If the path starts with "tmp/", it's relative to the backend directory
		if strings.HasPrefix(filePath, "tmp/") {
			// Convert to absolute path from backend directory for file existence check
			filePathForCheck := filepath.Join(".", filePath)

			// Check if file exists using the backend-relative path
			if _, err := os.Stat(filePathForCheck); os.IsNotExist(err) {
				log.Printf("❌ File not found: %s", filePathForCheck)
				respondWithError(w, "File not found", err)
				return
			}

			log.Printf("✅ File found: %s", filePathForCheck)

			// Convert to project-relative path for Python execution
			filePath = "backend/" + filePath
		} else {
			// Use the path as-is
			filePath = filepath.Clean(filePath)

			// Check if file exists
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				log.Printf("❌ File not found: %s", filePath)
				respondWithError(w, "File not found", err)
				return
			}

			log.Printf("✅ File found: %s", filePath)
		}
	} else {
		// Check if file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			log.Printf("❌ File not found: %s", filePath)
			respondWithError(w, "File not found", err)
			return
		}

		log.Printf("✅ File found: %s", filePath)
	}

	// Check if GEMINI_API_KEY is set
	if os.Getenv("GEMINI_API_KEY") == "" {
		log.Printf("❌ GEMINI_API_KEY environment variable not set")
		respondWithError(w, "GEMINI_API_KEY environment variable not set", nil)
		return
	}

	log.Printf("✅ GEMINI_API_KEY is configured")

	// Execute Python extraction script using virtual environment with absolute path
	pythonPath := "/Users/swaraj/Documents/vault/statementparser-poc/venv/bin/python"
	cmd := exec.Command(pythonPath, "rag/extract.py", filePath)

	// Set working directory to project root (one level up from backend)
	cmd.Dir = ".."

	// Load environment variables from .env file
	envVars, err := loadEnvVars("../.env")
	if err != nil {
		log.Printf("⚠️ Warning: Could not load .env file: %v", err)
	}
	cmd.Env = append(os.Environ(), envVars...)

	log.Printf("🚀 Starting Python extraction script: python rag/extract.py %s", filePath)
	log.Printf("📁 Working directory: %s", cmd.Dir)
	log.Printf("📄 Final file path: %s", filePath)
	log.Printf("🔧 Command: %s", cmd.String())

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Printf("⏳ Executing Python script...")

	// Add timeout to prevent hanging (increased for Gemini API calls)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	log.Printf("🔄 Starting command execution...")
	// Start the command and wait for completion or timeout
	err = cmd.Start()
	if err != nil {
		log.Printf("❌ Failed to start Python script: %v", err)
		respondWithError(w, "Failed to start Python script", err)
		return
	}
	log.Printf("✅ Python script started successfully (PID: %d)", cmd.Process.Pid)

	// Wait for completion or timeout
	done := make(chan error, 1)
	go func() {
		log.Printf("🔄 Waiting for Python script to complete...")
		done <- cmd.Wait()
		log.Printf("✅ Python script wait completed")
	}()

	log.Printf("🔄 Waiting for completion or timeout...")
	select {
	case err = <-done:
		// Command completed
		log.Printf("✅ Python script completed normally")
	case <-ctx.Done():
		// Timeout reached, kill the process
		log.Printf("⏰ Timeout reached, killing Python process...")
		cmd.Process.Kill()
		err = ctx.Err()
		log.Printf("⏰ Python script timed out after 180 seconds")
	}

	// Get the output
	output := stdout.Bytes()
	stderrOutput := stderr.Bytes()

	log.Printf("📊 Python script completed with exit code: %v", err)
	log.Printf("📝 Stdout length: %d bytes", len(output))
	log.Printf("⚠️  Stderr length: %d bytes", len(stderrOutput))

	if err != nil {
		log.Printf("❌ Python script failed with error: %v", err)
		log.Printf("📄 Stderr output: %s", string(stderrOutput))

		// Check if the output contains valid JSON despite the error
		var transactions []Transaction
		if json.Unmarshal(output, &transactions) == nil && len(transactions) > 0 {
			log.Printf("✅ Found valid JSON despite error, proceeding with %d transactions", len(transactions))
			// If we got valid JSON, ignore the error and proceed
			response := APIResponse{
				Success:      true,
				Message:      fmt.Sprintf("Successfully extracted %d transactions", len(transactions)),
				Transactions: transactions,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}

		log.Printf("❌ No valid JSON found in output")
		errorMsg := fmt.Sprintf("Failed to extract transactions: %v\nStdout: %s\nStderr: %s", err, string(output), string(stderrOutput))
		respondWithError(w, errorMsg, err)
		return
	}

	// Parse JSON output
	log.Printf("🔍 Parsing JSON output...")
	var transactions []Transaction
	if err := json.Unmarshal(output, &transactions); err != nil {
		log.Printf("❌ JSON parsing failed: %v", err)
		log.Printf("📄 Raw output: %s", string(output))
		// If JSON parsing fails, return raw output as error
		respondWithError(w, "Failed to parse JSON output", err)
		return
	}

	log.Printf("✅ Successfully parsed %d transactions", len(transactions))

	// Return successful response
	response := APIResponse{
		Success:      true,
		Message:      fmt.Sprintf("Successfully extracted %d transactions", len(transactions)),
		Transactions: transactions,
	}

	log.Printf("🎉 Sending successful response with %d transactions", len(transactions))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// healthHandler provides a health check endpoint
func healthHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"service":   "bank-rag-backend",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// respondWithError sends an error response
func respondWithError(w http.ResponseWriter, message string, err error) {
	errorMsg := message
	if err != nil {
		errorMsg = fmt.Sprintf("%s: %v", message, err)
	}

	response := APIResponse{
		Success: false,
		Message: message,
		Error:   errorMsg,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(response)
}

// corsMiddleware adds CORS headers
func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt gets an environment variable as integer with a default value
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// maskAPIKey masks the API key for logging (shows only first 8 and last 4 characters)
func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 12 {
		return "***masked***"
	}
	return apiKey[:8] + "..." + apiKey[len(apiKey)-4:]
}
