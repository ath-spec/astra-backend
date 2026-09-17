package bankparser

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/yourusername/astra-backend/internal/commons/logger"
)

// getMapKeys returns the keys of a map for debugging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// VeryfiClient handles communication with Veryfi API
type VeryfiClient struct {
	BaseURL      string
	ClientID     string
	APIKey       string
	ClientSecret string
	HTTPClient   *http.Client
}

// VeryfiConfig holds configuration for Veryfi client
type VeryfiConfig struct {
	BaseURL      string
	ClientID     string
	APIKey       string
	ClientSecret string
}

// NewVeryfiClient creates a new Veryfi client
func NewVeryfiClient(config VeryfiConfig) *VeryfiClient {
	// Validate configuration
	if config.BaseURL == "" {
		logger.Error("Veryfi client: BaseURL is empty")
	}
	if config.ClientID == "" {
		logger.Error("Veryfi client: ClientID is empty")
	}
	if config.APIKey == "" {
		logger.Error("Veryfi client: APIKey is empty")
	}

	logger.Info("Veryfi client: initialized with BaseURL=%s, ClientID=%s", config.BaseURL, config.ClientID)

	return &VeryfiClient{
		BaseURL:      config.BaseURL,
		ClientID:     config.ClientID,
		APIKey:       config.APIKey,
		ClientSecret: config.ClientSecret,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// generateSignature creates a signature for Veryfi API requests
func (c *VeryfiClient) generateSignature(payload []byte) (string, string) {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)

	// Create the signature string
	signatureString := fmt.Sprintf("%s:%s:%s", c.ClientID, timestamp, string(payload))

	// Generate HMAC-SHA256 signature
	h := hmac.New(sha256.New, []byte(c.ClientSecret))
	h.Write([]byte(signatureString))
	signature := hex.EncodeToString(h.Sum(nil))

	return signature, timestamp
}

// GetIDAsString converts the ID field to string regardless of its type
func GetIDAsString(id interface{}) string {
	switch v := id.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%.0f", v)
	case int:
		return fmt.Sprintf("%d", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// GetStringValue extracts string value from interface{} field
func GetStringValue(field interface{}) string {
	if field == nil {
		return ""
	}

	switch v := field.(type) {
	case string:
		return v
	case map[string]interface{}:
		// Handle Veryfi's complex field structure with confidence scores
		if value, ok := v["value"].(string); ok {
			return value
		}
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

// GetFloatValue extracts float64 value from interface{} field
func GetFloatValue(field interface{}) float64 {
	if field == nil {
		return 0.0
	}

	switch v := field.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case map[string]interface{}:
		// Handle Veryfi's complex field structure with confidence scores
		if value, ok := v["value"].(float64); ok {
			return value
		}
		if value, ok := v["value"].(int); ok {
			return float64(value)
		}
		return 0.0
	default:
		return 0.0
	}
}

// BankStatement represents a parsed bank statement from Veryfi
type BankStatement struct {
	ID              interface{} `json:"id"` // Can be string or number
	Created         string      `json:"created"`
	Updated         string      `json:"updated"`
	FileName        string      `json:"file_name"`
	FileSize        int         `json:"file_size"`
	FileType        string      `json:"file_type"`
	AccountNumber   interface{} `json:"account_number"` // Can be string or object
	AccountType     interface{} `json:"account_type"`   // Can be string or object
	BankName        interface{} `json:"bank_name"`      // Can be string or object
	RoutingNumber   interface{} `json:"routing_number"` // Can be string or object
	StatementPeriod struct {
		Start string `json:"start"`
		End   string `json:"end"`
	} `json:"statement_period"`
	TotalDeposits    interface{}            `json:"total_deposits"`    // Can be number or object
	TotalWithdrawals interface{}            `json:"total_withdrawals"` // Can be number or object
	BeginningBalance interface{}            `json:"beginning_balance"` // Can be number or object
	EndingBalance    interface{}            `json:"ending_balance"`    // Can be number or object
	Transactions     []Transaction          `json:"line_items"`
	RawData          map[string]interface{} `json:"-"`
}

// Transaction represents a single bank transaction
type Transaction struct {
	ID          string  `json:"id"`
	Date        string  `json:"date"`
	Description string  `json:"description"`
	Amount      float64 `json:"amount"`
	Type        string  `json:"type"`
	Category    string  `json:"category"`
	Reference   string  `json:"reference"`
	Balance     float64 `json:"balance"`
}

// UploadResponse represents the response from Veryfi upload
type UploadResponse struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Message  string `json:"message"`
	FileName string `json:"file_name"`
}

// ErrorResponse represents an error response from Veryfi
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// UploadDocument uploads a PDF file to Veryfi for processing
func (c *VeryfiClient) UploadDocument(filePath string) (*UploadResponse, error) {
	logger.Info("Veryfi upload: starting upload for file=%s", filePath)

	// Validate client configuration
	if c.BaseURL == "" || c.ClientID == "" || c.APIKey == "" {
		return nil, fmt.Errorf("veryfi client not properly configured: BaseURL=%s, ClientID=%s, APIKey=%s",
			c.BaseURL, c.ClientID, c.APIKey)
	}

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add file field
	fileWriter, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	_, err = io.Copy(fileWriter, file)
	if err != nil {
		return nil, fmt.Errorf("failed to copy file: %w", err)
	}

	// Add other required fields for bank statement processing
	writer.WriteField("file_name", filepath.Base(filePath))
	writer.WriteField("max_pages_to_process", "50")
	writer.WriteField("bounding_boxes", "true")
	writer.WriteField("confidence_details", "true")

	// Close the writer
	writer.Close()

	// Create request - using bank-statements endpoint
	url := fmt.Sprintf("%s/api/v8/partner/bank-statements", c.BaseURL)
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("CLIENT-ID", c.ClientID)
	req.Header.Set("AUTHORIZATION", fmt.Sprintf("apikey swaraj.chouriwar:%s", c.APIKey))

	// Log request details for debugging
	logger.Info("Veryfi upload: URL=%s, ClientID=%s, AuthHeader=%s",
		url, c.ClientID, fmt.Sprintf("apikey swaraj.chouriwar:%s", c.APIKey))

	// Make request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	logger.Info("Veryfi upload: response status=%d", resp.StatusCode)

	// Check for errors
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var errorResp ErrorResponse
		if err := json.Unmarshal(body, &errorResp); err == nil {
			// Handle empty error message
			if errorResp.Message == "" {
				errorResp.Message = "Unknown error from Veryfi API"
			}
			return nil, fmt.Errorf("veryfi api error: %s (code: %d)", errorResp.Message, errorResp.Code)
		}
		return nil, fmt.Errorf("veryfi api error: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response - bank statement endpoint returns BankStatement directly
	var bankStatement BankStatement
	if err := json.Unmarshal(body, &bankStatement); err != nil {
		return nil, fmt.Errorf("failed to parse bank statement response: %w", err)
	}

	// Store raw data for debugging
	bankStatement.RawData = make(map[string]interface{})
	json.Unmarshal(body, &bankStatement.RawData)

	documentID := GetIDAsString(bankStatement.ID)
	logger.Info("Veryfi upload: success document_id=%s, transactions=%d", documentID, len(bankStatement.Transactions))

	// Debug: Log raw response structure
	if len(bankStatement.Transactions) == 0 {
		logger.Warn("Veryfi upload: No transactions found. Raw response keys: %v", getMapKeys(bankStatement.RawData))
	}

	// Convert to UploadResponse for compatibility
	uploadResp := UploadResponse{
		ID:     documentID,
		Status: "success",
	}

	return &uploadResp, nil
}

// UploadDocumentWithFilename uploads a PDF file to Veryfi with a custom filename
func (c *VeryfiClient) UploadDocumentWithFilename(filePath string, customFilename string) (*UploadResponse, error) {
	logger.Info("Veryfi upload: starting upload for file=%s with custom filename=%s", filePath, customFilename)

	// Validate client configuration
	if c.BaseURL == "" || c.ClientID == "" || c.APIKey == "" {
		return nil, fmt.Errorf("veryfi client not properly configured: BaseURL=%s, ClientID=%s, APIKey=%s",
			c.BaseURL, c.ClientID, c.APIKey)
	}

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add file field with custom filename
	fileWriter, err := writer.CreateFormFile("file", customFilename)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	_, err = io.Copy(fileWriter, file)
	if err != nil {
		return nil, fmt.Errorf("failed to copy file: %w", err)
	}

	// Add other required fields for bank statement processing
	writer.WriteField("file_name", customFilename)
	writer.WriteField("max_pages_to_process", "50")
	writer.WriteField("bounding_boxes", "true")
	writer.WriteField("confidence_details", "true")

	// Close the writer
	writer.Close()

	// Create request - using bank-statements endpoint
	url := fmt.Sprintf("%s/api/v8/partner/bank-statements", c.BaseURL)
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("CLIENT-ID", c.ClientID)
	req.Header.Set("AUTHORIZATION", fmt.Sprintf("apikey swaraj.chouriwar:%s", c.APIKey))

	// Log request details for debugging
	logger.Info("Veryfi upload: URL=%s, ClientID=%s, CustomFilename=%s",
		url, c.ClientID, customFilename)

	// Make request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	logger.Info("Veryfi upload: response status=%d", resp.StatusCode)

	// Check for errors
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var errorResp ErrorResponse
		if err := json.Unmarshal(body, &errorResp); err == nil {
			// Handle empty error message
			if errorResp.Message == "" {
				errorResp.Message = "Unknown error from Veryfi API"
			}
			return nil, fmt.Errorf("veryfi api error: %s (code: %d)", errorResp.Message, errorResp.Code)
		}
		return nil, fmt.Errorf("veryfi api error: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response - bank statement endpoint returns BankStatement directly
	var bankStatement BankStatement
	if err := json.Unmarshal(body, &bankStatement); err != nil {
		return nil, fmt.Errorf("failed to parse bank statement response: %w", err)
	}

	// Store raw data for debugging
	bankStatement.RawData = make(map[string]interface{})
	json.Unmarshal(body, &bankStatement.RawData)

	documentID := GetIDAsString(bankStatement.ID)
	logger.Info("Veryfi upload: success document_id=%s with custom filename=%s, transactions=%d", documentID, customFilename, len(bankStatement.Transactions))

	// Debug: Log raw response structure
	if len(bankStatement.Transactions) == 0 {
		logger.Warn("Veryfi upload: No transactions found. Raw response keys: %v", getMapKeys(bankStatement.RawData))
	}

	// Convert to UploadResponse for compatibility
	uploadResp := UploadResponse{
		ID:     documentID,
		Status: "success",
	}

	return &uploadResp, nil
}

// GetDocument retrieves a processed document from Veryfi
func (c *VeryfiClient) GetDocument(documentID string) (*BankStatement, error) {
	logger.Info("Veryfi get: retrieving document_id=%s", documentID)

	url := fmt.Sprintf("%s/api/v8/partner/bank-statements/%s", c.BaseURL, documentID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("CLIENT-ID", c.ClientID)
	req.Header.Set("AUTHORIZATION", fmt.Sprintf("apikey swaraj.chouriwar:%s", c.APIKey))

	// Make request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	logger.Info("Veryfi get: response status=%d", resp.StatusCode)

	// Check for errors
	if resp.StatusCode != http.StatusOK {
		var errorResp ErrorResponse
		if err := json.Unmarshal(body, &errorResp); err == nil {
			return nil, fmt.Errorf("veryfi api error: %s (code: %d)", errorResp.Message, errorResp.Code)
		}
		return nil, fmt.Errorf("veryfi api error: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var bankStatement BankStatement
	if err := json.Unmarshal(body, &bankStatement); err != nil {
		return nil, fmt.Errorf("failed to parse bank statement: %w", err)
	}

	// Store raw data for debugging
	bankStatement.RawData = make(map[string]interface{})
	json.Unmarshal(body, &bankStatement.RawData)

	logger.Info("Veryfi get: success transactions=%d", len(bankStatement.Transactions))
	return &bankStatement, nil
}

// DeleteDocument deletes a document from Veryfi
func (c *VeryfiClient) DeleteDocument(documentID string) error {
	logger.Info("Veryfi delete: deleting document_id=%s", documentID)

	url := fmt.Sprintf("%s/api/v8/partner/bank-statements/%s", c.BaseURL, documentID)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("CLIENT-ID", c.ClientID)
	req.Header.Set("AUTHORIZATION", fmt.Sprintf("apikey swaraj.chouriwar:%s", c.APIKey))

	// Make request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	logger.Info("Veryfi delete: response status=%d", resp.StatusCode)

	// Check for errors
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("veryfi api error: status %d, body: %s", resp.StatusCode, string(body))
	}

	logger.Info("Veryfi delete: success document_id=%s", documentID)
	return nil
}

// ProcessDocument uploads a file, waits for processing, and returns the parsed data
func (c *VeryfiClient) ProcessDocument(filePath string) (*BankStatement, error) {
	// Upload document
	uploadResp, err := c.UploadDocument(filePath)
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}

	// Wait for processing (Veryfi processes documents asynchronously)
	// We'll poll the document status
	maxRetries := 30 // 30 seconds max wait
	for i := 0; i < maxRetries; i++ {
		time.Sleep(1 * time.Second)

		bankStatement, err := c.GetDocument(uploadResp.ID)
		if err != nil {
			// If it's a 404, document is still processing
			if strings.Contains(err.Error(), "404") {
				continue
			}
			return nil, fmt.Errorf("failed to get document: %w", err)
		}

		// Check if document is processed
		if GetIDAsString(bankStatement.ID) != "" {
			return bankStatement, nil
		}
	}

	return nil, fmt.Errorf("document processing timeout after %d seconds", maxRetries)
}

// ProcessDocumentWithFilename uploads a file with a custom filename to avoid caching
func (c *VeryfiClient) ProcessDocumentWithFilename(filePath string, customFilename string) (*BankStatement, error) {
	logger.Info("Veryfi process: using custom filename to avoid caching: %s", customFilename)

	// Upload document with custom filename
	uploadResp, err := c.UploadDocumentWithFilename(filePath, customFilename)
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}

	// Wait for processing (Veryfi processes documents asynchronously)
	// We'll poll the document status
	maxRetries := 30 // 30 seconds max wait
	for i := 0; i < maxRetries; i++ {
		time.Sleep(1 * time.Second)

		bankStatement, err := c.GetDocument(uploadResp.ID)
		if err != nil {
			// If it's a 404, document is still processing
			if strings.Contains(err.Error(), "404") {
				continue
			}
			return nil, fmt.Errorf("failed to get document: %w", err)
		}

		// Check if document is processed
		if GetIDAsString(bankStatement.ID) != "" {
			return bankStatement, nil
		}
	}

	return nil, fmt.Errorf("document processing timeout after %d seconds", maxRetries)
}
