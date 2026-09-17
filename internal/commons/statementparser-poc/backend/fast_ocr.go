package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// BankStatement represents the structured output
type BankStatement struct {
	AccountHolder string            `json:"account_holder"`
	AccountNumber string            `json:"account_number"`
	BankName      string            `json:"bank_name"`
	Transactions  []BankTransaction `json:"transactions"`
}

// BankTransaction represents a single transaction
type BankTransaction struct {
	Date        string  `json:"date"`
	Description string  `json:"description"`
	Amount      float64 `json:"amount"`
	Type        string  `json:"type"`
	Balance     float64 `json:"balance"`
}

// OCRResult represents PaddleOCR output
type OCRResult struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
	BBox       []int   `json:"bbox"`
}

// FastOCRPipeline handles the entire OCR pipeline
type FastOCRPipeline struct {
	tempDir string
}

// NewFastOCRPipeline creates a new OCR pipeline
func NewFastOCRPipeline() *FastOCRPipeline {
	tempDir := "backend/tmp/ocr"
	os.MkdirAll(tempDir, 0755)
	return &FastOCRPipeline{tempDir: tempDir}
}

// ProcessPDF processes a PDF and returns structured data
func (p *FastOCRPipeline) ProcessPDF(pdfPath string) (*BankStatement, error) {
	start := time.Now()
	defer func() {
		log.Printf("Total processing time: %v", time.Since(start))
	}()

	// Step 1: Detect PDF type
	isImageBased, err := p.detectPDFType(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to detect PDF type: %v", err)
	}

	log.Printf("PDF type: %s", map[bool]string{true: "image-based", false: "text-based"}[isImageBased])

	var text string
	if isImageBased {
		// Step 2: Convert PDF to images and run OCR
		text, err = p.processImageBasedPDF(pdfPath)
		if err != nil {
			return nil, fmt.Errorf("failed to process image-based PDF: %v", err)
		}
	} else {
		// Step 3: Extract text directly
		text, err = p.extractTextFromPDF(pdfPath)
		if err != nil {
			return nil, fmt.Errorf("failed to extract text: %v", err)
		}
	}

	log.Printf("Extracted text length: %d characters", len(text))

	// Step 4: Parse structured data
	statement, err := p.parseStructuredData(text)
	if err != nil {
		return nil, fmt.Errorf("failed to parse structured data: %v", err)
	}

	return statement, nil
}

// detectPDFType determines if PDF is text-based or image-based
func (p *FastOCRPipeline) detectPDFType(pdfPath string) (bool, error) {
	// Use pdftotext to check if PDF has extractable text
	cmd := exec.Command("pdftotext", "-l", "1", pdfPath, "-")
	output, err := cmd.Output()
	if err != nil {
		// If pdftotext fails, assume it's image-based
		return true, nil
	}

	// If we get less than 100 characters, it's likely image-based
	return len(strings.TrimSpace(string(output))) < 100, nil
}

// processImageBasedPDF converts PDF to images and runs OCR
func (p *FastOCRPipeline) processImageBasedPDF(pdfPath string) (string, error) {
	// Convert PDF to PNG images using pdftoppm
	outputPrefix := filepath.Join(p.tempDir, "page")
	cmd := exec.Command("pdftoppm", "-png", "-r", "300", pdfPath, outputPrefix)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to convert PDF to images: %v", err)
	}

	// Find all generated PNG files
	pngFiles, err := filepath.Glob(outputPrefix + "*.png")
	if err != nil {
		return "", fmt.Errorf("failed to find PNG files: %v", err)
	}

	var allText strings.Builder
	for _, pngFile := range pngFiles {
		// Run Tesseract OCR on each page
		pageText, err := p.runTesseractOCR(pngFile)
		if err != nil {
			log.Printf("Warning: OCR failed for %s: %v", pngFile, err)
			continue
		}
		allText.WriteString(pageText)
		allText.WriteString("\n")

		// Clean up PNG file
		os.Remove(pngFile)
	}

	return allText.String(), nil
}

// runTesseractOCR runs Tesseract OCR on an image file
func (p *FastOCRPipeline) runTesseractOCR(imagePath string) (string, error) {
	cmd := exec.Command("tesseract", imagePath, "stdout", "-l", "eng", "--psm", "6")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("Tesseract OCR failed: %v", err)
	}
	return string(output), nil
}

// extractTextFromPDF extracts text from text-based PDF
func (p *FastOCRPipeline) extractTextFromPDF(pdfPath string) (string, error) {
	cmd := exec.Command("pdftotext", pdfPath, "-")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to extract text: %v", err)
	}
	return string(output), nil
}

// parseStructuredData parses text into structured bank statement data
func (p *FastOCRPipeline) parseStructuredData(text string) (*BankStatement, error) {
	statement := &BankStatement{
		Transactions: []BankTransaction{},
	}

	lines := strings.Split(text, "\n")

	// Detect bank name
	statement.BankName = p.detectBankName(lines)

	// Extract account details
	statement.AccountHolder = p.extractAccountHolder(lines)
	statement.AccountNumber = p.extractAccountNumber(lines)

	// Parse transactions based on bank template
	switch statement.BankName {
	case "ICICI":
		statement.Transactions = p.parseICICITransactions(lines)
	case "HDFC":
		statement.Transactions = p.parseHDFCTransactions(lines)
	case "AXIS":
		statement.Transactions = p.parseAxisTransactions(lines)
	case "SBI":
		statement.Transactions = p.parseSBITransactions(lines)
	case "HSBC":
		statement.Transactions = p.parseHSBCTransactions(lines)
	default:
		// Generic parser for unknown banks
		statement.Transactions = p.parseGenericTransactions(lines)
	}

	return statement, nil
}

// detectBankName detects the bank name from text
func (p *FastOCRPipeline) detectBankName(lines []string) string {
	bankPatterns := map[string]*regexp.Regexp{
		"ICICI": regexp.MustCompile(`(?i)icici\s+bank`),
		"HDFC":  regexp.MustCompile(`(?i)hdfc\s+bank`),
		"AXIS":  regexp.MustCompile(`(?i)axis\s+bank`),
		"SBI":   regexp.MustCompile(`(?i)state\s+bank\s+of\s+india`),
		"HSBC":  regexp.MustCompile(`(?i)hsbc\s+bank`),
	}

	for bank, pattern := range bankPatterns {
		for _, line := range lines {
			if pattern.MatchString(line) {
				return bank
			}
		}
	}
	return "UNKNOWN"
}

// extractAccountHolder extracts account holder name
func (p *FastOCRPipeline) extractAccountHolder(lines []string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)account\s+holder[:\s]+([^\n]+)`),
		regexp.MustCompile(`(?i)name[:\s]+([^\n]+)`),
	}

	for _, pattern := range patterns {
		for _, line := range lines {
			matches := pattern.FindStringSubmatch(line)
			if len(matches) > 1 {
				return strings.TrimSpace(matches[1])
			}
		}
	}
	return ""
}

// extractAccountNumber extracts account number
func (p *FastOCRPipeline) extractAccountNumber(lines []string) string {
	pattern := regexp.MustCompile(`(?i)account\s+number[:\s]+(\d+)`)
	for _, line := range lines {
		matches := pattern.FindStringSubmatch(line)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	return ""
}

// parseICICITransactions parses ICICI Bank transactions
func (p *FastOCRPipeline) parseICICITransactions(lines []string) []BankTransaction {
	var transactions []BankTransaction

	// Enhanced patterns for better extraction
	datePattern := regexp.MustCompile(`(\d{1,2}/\d{1,2}/\d{4})`)       // DD/MM/YYYY format
	amountPattern := regexp.MustCompile(`(\d{1,3}(?:,\d{3})*\.\d{2})`) // 1,234.56 format
	typePattern := regexp.MustCompile(`\b(DR|CR)\b`)

	// Transaction keywords to look for (more specific)
	transactionKeywords := []string{
		"UPI/", "NEFT-", "IMPS", "AUTOPAY", "PAYTM", "ATM", "CASH", "TRANSFER",
		"MOBILE", "NET", "ONLINE", "POS", "SWIPE", "CARD", "BILL", "PAYMENT",
		"RECHARGE", "SUBSCRIBE", "FEE", "CHARGE", "INTEREST", "PENALTY",
	}

	// Header patterns to exclude
	headerPatterns := []string{
		"cheque number", "withdrawal", "deposit", "date", "description", "amount", "balance",
		"transaction", "particulars", "debit", "credit", "opening", "closing",
		"statement", "account", "period", "from", "to", "page", "total",
	}

	for i, line := range lines {
		lineUpper := strings.ToUpper(line)
		lineLower := strings.ToLower(line)

		// Skip header lines
		isHeader := false
		for _, header := range headerPatterns {
			if strings.Contains(lineLower, header) && len(line) < 50 {
				isHeader = true
				break
			}
		}

		if isHeader {
			continue
		}

		// Check if line contains any transaction keyword
		isTransaction := false
		for _, keyword := range transactionKeywords {
			if strings.Contains(lineUpper, keyword) {
				isTransaction = true
				break
			}
		}

		// Also check if line contains amount pattern AND has reasonable length
		if !isTransaction && amountPattern.MatchString(line) && len(line) > 10 && len(line) < 200 {
			// Make sure it's not just a number or header
			if !regexp.MustCompile(`^\d+\.\d{2}$`).MatchString(strings.TrimSpace(line)) {
				isTransaction = true
			}
		}

		if isTransaction {
			// Find date (look backwards up to 10 lines)
			date := ""
			for j := max(0, i-10); j < i; j++ {
				if matches := datePattern.FindStringSubmatch(lines[j]); len(matches) > 1 {
					date = matches[1]
					break
				}
			}

			// Find amount and type (look forwards up to 10 lines)
			amount := 0.0
			txType := "debit"
			balance := 0.0

			for j := i; j < min(len(lines), i+10); j++ {
				// Extract amount
				if matches := amountPattern.FindStringSubmatch(lines[j]); len(matches) > 1 {
					amountStr := strings.Replace(matches[1], ",", "", -1)
					if parsedAmount, err := strconv.ParseFloat(amountStr, 64); err == nil {
						amount = parsedAmount
					}
				}

				// Extract transaction type
				if matches := typePattern.FindStringSubmatch(lines[j]); len(matches) > 1 {
					if matches[1] == "CR" {
						txType = "credit"
					} else {
						txType = "debit"
					}
				}

				// Look for balance (usually after amount)
				balancePattern := regexp.MustCompile(`(\d{1,3}(?:,\d{3})*\.\d{2})`)
				if balanceMatches := balancePattern.FindAllStringSubmatch(lines[j], -1); len(balanceMatches) > 1 {
					// Take the second amount as balance (first is usually the transaction amount)
					if len(balanceMatches) > 1 {
						balanceStr := strings.Replace(balanceMatches[1][1], ",", "", -1)
						if parsedBalance, err := strconv.ParseFloat(balanceStr, 64); err == nil {
							balance = parsedBalance
						}
					}
				}
			}

			// Adjust amount sign based on transaction type
			if txType == "debit" && amount > 0 {
				amount = -amount
			}

			// Only add transaction if it has meaningful data and non-zero amount
			if amount != 0 {
				transaction := BankTransaction{
					Date:        date,
					Description: strings.TrimSpace(line),
					Amount:      amount,
					Type:        txType,
					Balance:     balance,
				}
				transactions = append(transactions, transaction)
			}
		}
	}

	// Deduplicate transactions based on description and amount
	deduplicated := make([]BankTransaction, 0)
	seen := make(map[string]bool)

	for _, transaction := range transactions {
		// Create a unique key based on description and amount
		key := fmt.Sprintf("%s|%.2f", transaction.Description, transaction.Amount)

		if !seen[key] {
			seen[key] = true
			deduplicated = append(deduplicated, transaction)
		}
	}

	return deduplicated
}

// parseHDFCTransactions parses HDFC Bank transactions with comprehensive OCR-aware parsing
func (p *FastOCRPipeline) parseHDFCTransactions(lines []string) []BankTransaction {
	var transactions []BankTransaction

	// Clean and process lines
	processedLines := p.cleanAndProcessLines(lines)

	// Find all transaction blocks
	transactionBlocks := p.findTransactionBlocks(processedLines)

	// Parse each transaction block
	for _, block := range transactionBlocks {
		transaction := p.parseTransactionBlock(block)
		if transaction.Amount != 0 {
			transactions = append(transactions, transaction)
		}
	}

	return transactions
}

// cleanAndProcessLines cleans OCR output and processes lines
func (p *FastOCRPipeline) cleanAndProcessLines(lines []string) []string {
	var processed []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Skip header lines
		if strings.Contains(strings.ToUpper(line), "ACCOUNT NUMBER") ||
			strings.Contains(strings.ToUpper(line), "TRANSACTION DATE") ||
			strings.Contains(strings.ToUpper(line), "DESCRIPTION") ||
			strings.Contains(strings.ToUpper(line), "AMOUNT") ||
			strings.Contains(strings.ToUpper(line), "TYPE") ||
			strings.Contains(strings.ToUpper(line), "SYSTEM-GENERATED") {
			continue
		}

		processed = append(processed, line)
	}

	return processed
}

// findTransactionBlocks finds blocks of lines that belong to the same transaction
func (p *FastOCRPipeline) findTransactionBlocks(lines []string) [][]string {
	var blocks [][]string
	var currentBlock []string

	for _, line := range lines {
		// Check if this line starts a new transaction
		if p.isTransactionStart(line) {
			// Save previous block if it exists
			if len(currentBlock) > 0 {
				blocks = append(blocks, currentBlock)
			}
			currentBlock = []string{line}
		} else {
			// This is a continuation line
			if len(currentBlock) > 0 {
				currentBlock = append(currentBlock, line)
			} else {
				// Start a new block
				currentBlock = []string{line}
			}
		}
	}

	// Add the last block
	if len(currentBlock) > 0 {
		blocks = append(blocks, currentBlock)
	}

	return blocks
}

// isTransactionStart checks if a line starts a new transaction
func (p *FastOCRPipeline) isTransactionStart(line string) bool {
	// Check for date pattern at the start
	datePattern := regexp.MustCompile(`^\d{1,2}-\d{1,2}-\d{4}`)
	if datePattern.MatchString(line) {
		return true
	}

	// Check for transaction keywords (expanded list)
	keywords := []string{
		"UPI/", "NEFT-", "IMPS", "AUTOPAY", "PAYTM", "ATM", "CASH", "TRANSFER",
		"DEBIT", "CREDIT", "WITHDRAWAL", "DEPOSIT", "CHEQUE", "DD", "RTGS",
		"MOBILE", "NET", "ONLINE", "POS", "SWIPE", "CARD", "BILL", "PAYMENT",
		"RECHARGE", "SUBSCRIBE", "FEE", "CHARGE", "INTEREST", "PENALTY",
	}
	for _, keyword := range keywords {
		if strings.Contains(strings.ToUpper(line), keyword) {
			return true
		}
	}

	// Check if line contains amount pattern (likely a transaction)
	amountPattern := regexp.MustCompile(`\d+\.\d{2}`)
	if amountPattern.MatchString(line) && len(line) > 10 && len(line) < 200 {
		return true
	}

	return false
}

// parseTransactionBlock parses a block of lines into a single transaction
func (p *FastOCRPipeline) parseTransactionBlock(block []string) BankTransaction {
	transaction := BankTransaction{
		Date:        "",
		Description: "",
		Amount:      0,
		Type:        "debit",
		Balance:     0,
	}

	// Join all lines in the block
	fullText := strings.Join(block, " ")

	// Extract date - try multiple formats
	datePatterns := []*regexp.Regexp{
		regexp.MustCompile(`(\d{1,2}/\d{1,2}/\d{4})`),   // DD/MM/YYYY
		regexp.MustCompile(`(\d{1,2}-\d{1,2}-\d{4})`),   // DD-MM-YYYY
		regexp.MustCompile(`(\d{1,2}\.\d{1,2}\.\d{4})`), // DD.MM.YYYY
	}

	for _, pattern := range datePatterns {
		if matches := pattern.FindStringSubmatch(fullText); len(matches) > 1 {
			transaction.Date = matches[1]
			break
		}
	}

	// Extract amount - look for proper amount patterns
	amountPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(\d{1,3}(?:,\d{3})*\.\d{2})`), // 1,234.56
		regexp.MustCompile(`(\d+\.\d{2})`),                // 123.45
		regexp.MustCompile(`(\d+)`),                       // 123
	}

	for _, pattern := range amountPatterns {
		if matches := pattern.FindStringSubmatch(fullText); len(matches) > 1 {
			amountStr := strings.Replace(matches[1], ",", "", -1)
			if amount, err := strconv.ParseFloat(amountStr, 64); err == nil {
				transaction.Amount = amount
				break
			}
		}
	}

	// Extract balance - look for balance patterns
	balancePatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)bal[ance]*\s*:?\s*(\d{1,3}(?:,\d{3})*\.\d{2})`),
		regexp.MustCompile(`(?i)balance\s*:?\s*(\d+\.\d{2})`),
	}

	for _, pattern := range balancePatterns {
		if matches := pattern.FindStringSubmatch(fullText); len(matches) > 1 {
			balanceStr := strings.Replace(matches[1], ",", "", -1)
			if balance, err := strconv.ParseFloat(balanceStr, 64); err == nil {
				transaction.Balance = balance
				break
			}
		}
	}

	// Determine transaction type and adjust amount sign
	if strings.Contains(strings.ToUpper(fullText), "CR") || strings.Contains(strings.ToUpper(fullText), "CREDIT") {
		transaction.Type = "credit"
		// Keep positive amount for credit
	} else {
		transaction.Type = "debit"
		// Make amount negative for debit
		if transaction.Amount > 0 {
			transaction.Amount = -transaction.Amount
		}
	}

	// Build description - clean up the text
	transaction.Description = strings.TrimSpace(fullText)

	// Remove excessive whitespace
	transaction.Description = regexp.MustCompile(`\s+`).ReplaceAllString(transaction.Description, " ")

	return transaction
}

// parseHDFCLineByLine fallback parser for HDFC transactions
func (p *FastOCRPipeline) parseHDFCLineByLine(lines []string) []BankTransaction {
	var transactions []BankTransaction

	datePattern := regexp.MustCompile(`(\d{1,2}-\d{1,2}-\d{4})`)
	amountPattern := regexp.MustCompile(`(\d+\.\d{2})`)
	typePattern := regexp.MustCompile(`\b(DR|CR)\b`)
	upiPattern := regexp.MustCompile(`(?i)upi`)

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check if line contains UPI
		if upiPattern.MatchString(line) {
			// Find date (look backwards up to 5 lines)
			date := ""
			for j := max(0, i-5); j < i; j++ {
				if matches := datePattern.FindStringSubmatch(lines[j]); len(matches) > 1 {
					date = matches[1]
					break
				}
			}

			if date == "" {
				date = "2025-01-01"
			}

			// Find amount and type (look forwards up to 5 lines)
			amount := 0.0
			txType := "debit"

			for j := i; j < min(len(lines), i+5); j++ {
				if matches := amountPattern.FindStringSubmatch(lines[j]); len(matches) > 1 {
					amount, _ = strconv.ParseFloat(matches[1], 64)
				}
				if matches := typePattern.FindStringSubmatch(lines[j]); len(matches) > 1 {
					if matches[1] == "CR" {
						txType = "credit"
					} else {
						txType = "debit"
					}
					break
				}
			}

			if txType == "debit" {
				amount = -amount
			}

			if amount != 0 {
				transaction := BankTransaction{
					Date:        date,
					Description: line,
					Amount:      amount,
					Type:        txType,
					Balance:     0,
				}
				transactions = append(transactions, transaction)
			}
		}
	}

	return transactions
}

// reconstructHDFCLines reconstructs fragmented OCR lines into complete transaction lines
func (p *FastOCRPipeline) reconstructHDFCLines(lines []string) []string {
	var reconstructed []string
	var currentLine strings.Builder

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if currentLine.Len() > 0 {
				reconstructed = append(reconstructed, currentLine.String())
				currentLine.Reset()
			}
			continue
		}

		// Check if this line starts a new transaction
		if p.isNewTransactionStart(line) {
			// Save previous line if it exists
			if currentLine.Len() > 0 {
				reconstructed = append(reconstructed, currentLine.String())
				currentLine.Reset()
			}
			currentLine.WriteString(line)
		} else {
			// This is a continuation line - check if it's part of a transaction
			if currentLine.Len() > 0 && p.isTransactionContinuation(line) {
				currentLine.WriteString(" " + line)
			} else if currentLine.Len() > 0 {
				// End current transaction and start new one
				reconstructed = append(reconstructed, currentLine.String())
				currentLine.Reset()
				currentLine.WriteString(line)
			} else {
				currentLine.WriteString(line)
			}
		}
	}

	// Add the last line if it exists
	if currentLine.Len() > 0 {
		reconstructed = append(reconstructed, currentLine.String())
	}

	return reconstructed
}

// isTransactionContinuation checks if a line is a continuation of a transaction
func (p *FastOCRPipeline) isTransactionContinuation(line string) bool {
	// Check for patterns that indicate continuation
	continuationPatterns := []string{
		"BANK/", "LTD/", "@", "/", "PTM", "AXI", "SB", "HDFC", "STATE",
		"CR", "DR", "NEFT", "IMPS", "AUTOPAY", "PAYTM",
	}

	for _, pattern := range continuationPatterns {
		if strings.Contains(strings.ToUpper(line), pattern) {
			return true
		}
	}

	// Check if line contains only numbers, letters, and common transaction characters
	if len(line) > 0 && len(line) < 100 {
		hasValidChars := false
		for _, char := range line {
			if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') ||
				(char >= '0' && char <= '9') || char == '/' || char == '@' ||
				char == '.' || char == '-' || char == '_' {
				hasValidChars = true
			} else if char != ' ' {
				return false
			}
		}
		return hasValidChars
	}

	return false
}

// isNewTransactionStart checks if a line starts a new transaction
func (p *FastOCRPipeline) isNewTransactionStart(line string) bool {
	// Check for date pattern at the start
	datePattern := regexp.MustCompile(`^\d{1,2}-\d{1,2}-\d{4}`)
	if datePattern.MatchString(line) {
		return true
	}

	// Check for transaction keywords
	keywords := []string{"UPI/", "NEFT-", "IMPS", "AUTOPAY", "PAYTM"}
	for _, keyword := range keywords {
		if strings.Contains(strings.ToUpper(line), keyword) {
			return true
		}
	}

	// Check if line contains amount pattern (likely a standalone transaction)
	amountPattern := regexp.MustCompile(`\d+\.\d{2}`)
	if amountPattern.MatchString(line) && len(line) < 50 {
		return true
	}

	return false
}

// parseAxisTransactions parses Axis Bank transactions
func (p *FastOCRPipeline) parseAxisTransactions(lines []string) []BankTransaction {
	var transactions []BankTransaction

	// Axis Bank UPI pattern: UPI/(P2M|P2A)/\d{9,}/([^/]+)/.*?(?:v|Pay(?: to|men)?)/\s*([A-Z ]+?)(?:\s+LTD|LIMITED|BANK|FINANC|$)
	axisUPIPattern := regexp.MustCompile(`(?i)UPI/(P2M|P2A)/\d{9,}/([^/]+)/.*?(?:v|Pay(?: to|men)?)/\s*([A-Z ]+?)(?:\s+LTD|LIMITED|BANK|FINANC|$)`)
	amountPattern := regexp.MustCompile(`(\d+\.\d{2})`)

	for i, line := range lines {
		if matches := axisUPIPattern.FindStringSubmatch(line); len(matches) >= 4 {
			// Extract amount from nearby lines
			amount := 0.0
			for j := i; j < min(len(lines), i+5); j++ {
				if amtMatches := amountPattern.FindStringSubmatch(lines[j]); len(amtMatches) > 1 {
					amount, _ = strconv.ParseFloat(amtMatches[1], 64)
					break
				}
			}

			// Build enhanced description
			txType := matches[1] // P2M or P2A
			payee := matches[2]  // Payee name
			bank := matches[3]   // Bank name

			description := fmt.Sprintf("UPI %s to %s via %s", txType, payee, bank)

			transaction := BankTransaction{
				Date:        "2025-01-01",
				Description: description,
				Amount:      -amount,
				Type:        "debit",
				Balance:     0,
			}
			transactions = append(transactions, transaction)
		} else if strings.Contains(strings.ToUpper(line), "UPI") {
			// Fallback to generic UPI parsing
			amount := 0.0
			for j := i; j < min(len(lines), i+5); j++ {
				if matches := amountPattern.FindStringSubmatch(lines[j]); len(matches) > 1 {
					amount, _ = strconv.ParseFloat(matches[1], 64)
					break
				}
			}

			transaction := BankTransaction{
				Date:        "2025-01-01",
				Description: strings.TrimSpace(line),
				Amount:      -amount,
				Type:        "debit",
				Balance:     0,
			}
			transactions = append(transactions, transaction)
		}
	}

	return transactions
}

// parseSBITransactions parses SBI transactions
func (p *FastOCRPipeline) parseSBITransactions(lines []string) []BankTransaction {
	var transactions []BankTransaction

	// SBI Bank UPI pattern: UPI\/(?:DR|CR)\/(\d+)\/([\w\s.&-]+)\/([A-Z]{4})\/([a-zA-Z0-9._@-]+|[a-zA-Z0-9._-]+)\/?([\w\s-]*)?
	sbiUPIPattern := regexp.MustCompile(`UPI\/(?:DR|CR)\/(\d+)\/([\w\s.&-]+)\/([A-Z]{4})\/([a-zA-Z0-9._@-]+|[a-zA-Z0-9._-]+)\/?([\w\s-]*)?`)
	amountPattern := regexp.MustCompile(`(\d+\.\d{2})`)

	for i, line := range lines {
		if matches := sbiUPIPattern.FindStringSubmatch(line); len(matches) >= 6 {
			// Extract amount from nearby lines
			amount := 0.0
			for j := i; j < min(len(lines), i+5); j++ {
				if amtMatches := amountPattern.FindStringSubmatch(lines[j]); len(amtMatches) > 1 {
					amount, _ = strconv.ParseFloat(amtMatches[1], 64)
					break
				}
			}

			// Build enhanced description
			merchant := matches[2] // Merchant name
			bankCode := matches[3] // Bank code
			desc := matches[5]     // Description

			description := fmt.Sprintf("UPI to %s via %s", merchant, bankCode)
			if desc != "" && desc != merchant {
				description += fmt.Sprintf(" - %s", desc)
			}

			transaction := BankTransaction{
				Date:        "2025-01-01",
				Description: description,
				Amount:      -amount,
				Type:        "debit",
				Balance:     0,
			}
			transactions = append(transactions, transaction)
		} else if strings.Contains(strings.ToUpper(line), "UPI") {
			// Fallback to generic UPI parsing
			amount := 0.0
			for j := i; j < min(len(lines), i+5); j++ {
				if matches := amountPattern.FindStringSubmatch(lines[j]); len(matches) > 1 {
					amount, _ = strconv.ParseFloat(matches[1], 64)
					break
				}
			}

			transaction := BankTransaction{
				Date:        "2025-01-01",
				Description: strings.TrimSpace(line),
				Amount:      -amount,
				Type:        "debit",
				Balance:     0,
			}
			transactions = append(transactions, transaction)
		}
	}

	return transactions
}

// parseHSBCTransactions parses HSBC Bank transactions
func (p *FastOCRPipeline) parseHSBCTransactions(lines []string) []BankTransaction {
	var transactions []BankTransaction

	// HSBC Bank UPI pattern: UPI\d{17}\s+\d{9,}\s+([A-Z][A-Za-z .]+(?:Limited|Ltd|PRIVATE|PVT)?)
	hsbcUPIPattern := regexp.MustCompile(`(?i)UPI\d{17}\s+\d{9,}\s+([A-Z][A-Za-z .]+(?:Limited|Ltd|PRIVATE|PVT)?)`)
	amountPattern := regexp.MustCompile(`(\d+\.\d{2})`)

	for i, line := range lines {
		if matches := hsbcUPIPattern.FindStringSubmatch(line); len(matches) >= 2 {
			// Extract amount from nearby lines
			amount := 0.0
			for j := i; j < min(len(lines), i+5); j++ {
				if amtMatches := amountPattern.FindStringSubmatch(lines[j]); len(amtMatches) > 1 {
					amount, _ = strconv.ParseFloat(amtMatches[1], 64)
					break
				}
			}

			// Build enhanced description
			payee := strings.TrimSpace(matches[1]) // Payee/Merchant name

			description := fmt.Sprintf("UPI payment to %s", payee)

			transaction := BankTransaction{
				Date:        "2025-01-01",
				Description: description,
				Amount:      -amount,
				Type:        "debit",
				Balance:     0,
			}
			transactions = append(transactions, transaction)
		} else if strings.Contains(strings.ToUpper(line), "UPI") {
			// Fallback to generic UPI parsing
			amount := 0.0
			for j := i; j < min(len(lines), i+5); j++ {
				if matches := amountPattern.FindStringSubmatch(lines[j]); len(matches) > 1 {
					amount, _ = strconv.ParseFloat(matches[1], 64)
					break
				}
			}

			transaction := BankTransaction{
				Date:        "2025-01-01",
				Description: strings.TrimSpace(line),
				Amount:      -amount,
				Type:        "debit",
				Balance:     0,
			}
			transactions = append(transactions, transaction)
		}
	}

	return transactions
}

// parseGenericTransactions generic parser for unknown banks
func (p *FastOCRPipeline) parseGenericTransactions(lines []string) []BankTransaction {
	var transactions []BankTransaction

	upiPattern := regexp.MustCompile(`(?i)upi`)
	amountPattern := regexp.MustCompile(`(\d+\.\d{2})`)

	for i, line := range lines {
		if upiPattern.MatchString(line) {
			// Look for amount in nearby lines
			amount := 0.0
			for j := i; j < min(len(lines), i+5); j++ {
				if matches := amountPattern.FindStringSubmatch(lines[j]); len(matches) > 1 {
					amount, _ = strconv.ParseFloat(matches[1], 64)
					break
				}
			}

			transaction := BankTransaction{
				Date:        "2025-01-01", // Default date
				Description: strings.TrimSpace(line),
				Amount:      -amount, // Assume debit for UPI
				Type:        "debit",
				Balance:     0,
			}
			transactions = append(transactions, transaction)
		}
	}

	return transactions
}

// Helper functions
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// FastExtractHandler handles the fast extraction endpoint
func fastExtractHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filePath := r.URL.Query().Get("file")
	if filePath == "" {
		respondWithError(w, "Missing 'file' query parameter", nil)
		return
	}

	log.Printf("🚀 Fast extraction request for: %s", filePath)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		respondWithError(w, "File not found", err)
		return
	}

	// Process PDF with fast OCR pipeline
	pipeline := NewFastOCRPipeline()
	statement, err := pipeline.ProcessPDF(filePath)
	if err != nil {
		respondWithError(w, "Failed to process PDF", err)
		return
	}

	// Return structured response
	response := map[string]interface{}{
		"success":         true,
		"message":         fmt.Sprintf("Successfully extracted %d transactions", len(statement.Transactions)),
		"statement":       statement,
		"processing_time": "fast",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
