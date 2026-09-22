package statementparser

import (
	"context"
	"fmt"
	"time"
)

// StatementParserClient handles communication with the statementparser service
type StatementParserClient struct {
	embeddedHandler EmbeddedHandler
	useEmbedded     bool
}

// EmbeddedHandler interface for embedded statementparser functionality
type EmbeddedHandler interface {
	HealthCheck() (*HealthCheckResponse, error)
	UploadDocument(filePath string) (*UploadDocumentResponse, error)
	ExtractTransactions(filePath string) (*ExtractTransactionsResponse, error)
	FastExtractTransactions(filePath string) (*FastExtractTransactionsResponse, error)
}

// StatementParserConfig represents the configuration for StatementParserClient
type StatementParserConfig struct {
	BaseURL         string
	Timeout         time.Duration
	StorageProvider string
	MaxFileSizeMB   int
	TempDir         string
	LogLevel        string
	DebugMode       bool
	EmbeddedHandler EmbeddedHandler // New field for embedded handler
}

// NewStatementParserClient creates a new StatementParserClient
func NewStatementParserClient(config StatementParserConfig) *StatementParserClient {
	useEmbedded := config.EmbeddedHandler != nil

	return &StatementParserClient{
		embeddedHandler: config.EmbeddedHandler,
		useEmbedded:     useEmbedded,
	}
}

// HealthCheck performs a health check on the statementparser service
func (c *StatementParserClient) HealthCheck() (*HealthCheckResponse, error) {
	if c.useEmbedded {
		return c.embeddedHandler.HealthCheck()
	}

	// Fallback to mock response if no embedded handler
	return &HealthCheckResponse{
		Status:  "ok",
		Message: "StatementParser service is running (mock)",
	}, nil
}

// UploadDocument uploads a document to the statementparser service
func (c *StatementParserClient) UploadDocument(filePath string) (*UploadDocumentResponse, error) {
	if c.useEmbedded {
		return c.embeddedHandler.UploadDocument(filePath)
	}

	// Fallback to mock response if no embedded handler
	return &UploadDocumentResponse{
		Success:    true,
		Message:    "Document uploaded successfully (mock)",
		DocumentID: "doc_mock_123",
	}, nil
}

// ExtractTransactions extracts transactions from a document using file path
func (c *StatementParserClient) ExtractTransactions(filePath string) (*ExtractTransactionsResponse, error) {
	if c.useEmbedded {
		return c.embeddedHandler.ExtractTransactions(filePath)
	}

	// Fallback to mock response if no embedded handler
	return &ExtractTransactionsResponse{
		Success: true,
		Error:   "",
		BankStatement: &BankStatement{
			AccountHolder: "Mock Account",
			AccountNumber: "1234567890",
			BankName:      "Mock Bank",
			Transactions:  []BankTransaction{},
		},
		Transactions: []BankTransaction{},
	}, nil
}

// FastExtractTransactions performs fast transaction extraction
func (c *StatementParserClient) FastExtractTransactions(filePath string) (*FastExtractTransactionsResponse, error) {
	if c.useEmbedded {
		return c.embeddedHandler.FastExtractTransactions(filePath)
	}

	// Fallback to mock response if no embedded handler
	return &FastExtractTransactionsResponse{
		Success: true,
		Error:   "",
		Statement: &BankStatement{
			AccountHolder: "Mock Account",
			AccountNumber: "1234567890",
			BankName:      "Mock Bank",
			Transactions:  []BankTransaction{},
		},
		ProcessingTime: "fast",
	}, nil
}

// HealthCheckResponse represents the health check response
type HealthCheckResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// UploadDocumentResponse represents the document upload response
type UploadDocumentResponse struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	DocumentID string `json:"document_id"`
}

// ExtractTransactionsResponse represents the transaction extraction response
type ExtractTransactionsResponse struct {
	Success       bool              `json:"success"`
	Error         string            `json:"error"`
	BankStatement *BankStatement    `json:"bank_statement"`
	Transactions  []BankTransaction `json:"transactions"`
}

// FastExtractTransactionsResponse represents the fast transaction extraction response
type FastExtractTransactionsResponse struct {
	Success        bool           `json:"success"`
	Error          string         `json:"error"`
	Statement      *BankStatement `json:"statement"`
	ProcessingTime string         `json:"processing_time"`
}

// BankStatement represents a bank statement with transactions
type BankStatement struct {
	AccountHolder string            `json:"account_holder"`
	AccountNumber string            `json:"account_number"`
	BankName      string            `json:"bank_name"`
	Transactions  []BankTransaction `json:"transactions"`
}

// Transaction represents a transaction from statementparser-poc
type Transaction struct {
	Date            string  `json:"date"`
	Description     string  `json:"description"`
	Amount          float64 `json:"amount"`
	Balance         float64 `json:"balance"`
	TransactionType string  `json:"transaction_type"`
}

// BankTransaction represents a single bank transaction
type BankTransaction struct {
	ID          string    `json:"id"`
	Date        time.Time `json:"date"`
	Description string    `json:"description"`
	Amount      float64   `json:"amount"`
	Type        string    `json:"type"`
	Category    string    `json:"category"`
	Balance     float64   `json:"balance"`
}

// parseDate parses date string to time.Time
func parseDate(dateStr string) time.Time {
	// Try different date formats
	formats := []string{
		"2006-01-02",
		"02/01/2006",
		"02-01-2006",
		"2006/01/02",
		"Jan 02, 2006",
		"02 Jan 2006",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t
		}
	}

	// If all formats fail, return current time
	return time.Now()
}

// ProcessDocument processes a document through the statementparser service
func (c *StatementParserClient) ProcessDocument(ctx context.Context, filePath string) (*ProcessDocumentResponse, error) {
	// First upload the document
	uploadResp, err := c.UploadDocument(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to upload document: %w", err)
	}

	// Then extract transactions
	extractResp, err := c.ExtractTransactions(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract transactions: %w", err)
	}

	return &ProcessDocumentResponse{
		Success: uploadResp.Success && extractResp.Success,
		Message: fmt.Sprintf("Document processed successfully. Upload: %s, Extract: %d transactions", uploadResp.Message, len(extractResp.Transactions)),
	}, nil
}

// ProcessDocumentResponse represents the document processing response
type ProcessDocumentResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
