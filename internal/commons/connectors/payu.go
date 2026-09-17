package connectors

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/yourusername/astra-backend/internal/commons/logger"
)

// PayU API constants
const (
	PayUAPIURL     = "https://info.payu.in/merchant/postservice?form=2"
	PayUCommand    = "verify_payment"
	PayUTimeoutSec = 30
)

// PayUConfig holds PayU API configuration
type PayUConfig struct {
	MerchantKey string
	Salt        string
	APIURL      string
	Timeout     time.Duration
}

// PayUResponse represents the response from PayU API
type PayUResponse struct {
	Status             string `json:"status"`
	Msg                string `json:"msg"`
	TransactionDetails struct {
		Mihpayid           string `json:"mihpayid"`
		Mode               string `json:"mode"`
		Status             string `json:"status"`
		Unmappedstatus     string `json:"unmappedstatus"`
		Key                string `json:"key"`
		Txnid              string `json:"txnid"`
		Amount             string `json:"amount"`
		CardCategory       string `json:"cardCategory"`
		Discount           string `json:"discount"`
		NetAmountDebit     string `json:"net_amount_debit"`
		Addedon            string `json:"addedon"`
		Productinfo        string `json:"productinfo"`
		Firstname          string `json:"firstname"`
		Lastname           string `json:"lastname"`
		Address1           string `json:"address1"`
		Address2           string `json:"address2"`
		City               string `json:"city"`
		State              string `json:"state"`
		Country            string `json:"country"`
		Zipcode            string `json:"zipcode"`
		Email              string `json:"email"`
		Phone              string `json:"phone"`
		Hash               string `json:"hash"`
		Field1             string `json:"field1"`
		Field2             string `json:"field2"`
		Field3             string `json:"field3"`
		Field4             string `json:"field4"`
		Field5             string `json:"field5"`
		Field6             string `json:"field6"`
		Field7             string `json:"field7"`
		Field8             string `json:"field8"`
		Field9             string `json:"field9"`
		EncryptedPaymentID string `json:"encryptedPaymentId"`
		BankRefNum         string `json:"bank_ref_num"`
		Bankcode           string `json:"bankcode"`
		Error              string `json:"error"`
		ErrorMessage       string `json:"error_Message"`
		IsConsentPayment   string `json:"isConsentPayment"`
	} `json:"transaction_details"`
}

// PayUClient handles PayU API interactions
type PayUClient struct {
	config PayUConfig
	client *http.Client
}

// NewPayUClient creates a new PayU client
func NewPayUClient(config PayUConfig) *PayUClient {
	if config.Timeout == 0 {
		config.Timeout = PayUTimeoutSec * time.Second
	}
	if config.APIURL == "" {
		config.APIURL = PayUAPIURL
	}

	return &PayUClient{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// VerifyPayment verifies a payment transaction with PayU
func (p *PayUClient) VerifyPayment(ctx context.Context, txnID string) (*PayUResponse, error) {
	if txnID == "" {
		return nil, fmt.Errorf("transaction ID cannot be empty")
	}

	// Generate hash for PayU API
	hash := p.generateHash(txnID)

	// Prepare form data
	data := url.Values{}
	data.Set("key", p.config.MerchantKey)
	data.Set("command", PayUCommand)
	data.Set("var1", txnID)
	data.Set("hash", hash)

	// Create HTTP request with context
	req, err := http.NewRequestWithContext(ctx, "POST", p.config.APIURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	logger.Info("Making PayU verification request for txnId: %s", txnID)

	// Make HTTP request
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	var payuResp PayUResponse
	if err := json.NewDecoder(resp.Body).Decode(&payuResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	logger.Info("PayU verification response for txnId %s: status=%s, transaction_status=%s",
		txnID, payuResp.Status, payuResp.TransactionDetails.Status)

	return &payuResp, nil
}

// IsPaymentSuccessful checks if the payment was successful
func (p *PayUClient) IsPaymentSuccessful(response *PayUResponse) bool {
	return response.Status == "success" && response.TransactionDetails.Status == "success"
}

// generateHash generates SHA512 hash for PayU API authentication
func (p *PayUClient) generateHash(txnID string) string {
	// Create hash string: key|command|txnId|salt
	hashString := fmt.Sprintf("%s|%s|%s|%s", p.config.MerchantKey, PayUCommand, txnID, p.config.Salt)

	// Generate SHA512 hash
	hash := sha512.Sum512([]byte(hashString))

	// Return as hex string
	return fmt.Sprintf("%x", hash)
}

// VerifyPaymentWithRetry verifies payment with retry logic
func (p *PayUClient) VerifyPaymentWithRetry(ctx context.Context, txnID string, maxRetries int) (*PayUResponse, error) {
	var lastErr error

	for i := 0; i <= maxRetries; i++ {
		if i > 0 {
			logger.Info("Retrying PayU verification for txnId %s (attempt %d/%d)", txnID, i+1, maxRetries+1)
			time.Sleep(time.Second * time.Duration(i)) // Exponential backoff
		}

		resp, err := p.VerifyPayment(ctx, txnID)
		if err == nil {
			return resp, nil
		}

		lastErr = err
		logger.Error("PayU verification attempt %d failed for txnId %s: %v", i+1, txnID, err)
	}

	return nil, fmt.Errorf("PayU verification failed after %d attempts: %w", maxRetries+1, lastErr)
}
