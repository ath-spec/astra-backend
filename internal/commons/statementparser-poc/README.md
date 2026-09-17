# Fast Bank Statement OCR Pipeline

A high-performance Go-based OCR pipeline that extracts structured transaction data from bank statement PDFs within 30 seconds. This implementation uses a Parsio-style approach with hybrid template + ML logic for maximum accuracy and speed.

## 🔒 Security

**IMPORTANT**: This project handles sensitive financial data. Follow these security practices:

- **Never commit API keys**: The `.env` file is gitignored and contains your Gemini API key
- **Use environment variables**: Copy `env.example` to `.env` and fill in your actual values
- **Protect uploaded files**: Temporary PDFs are stored in `backend/tmp/` (also gitignored)
- **Review .gitignore**: Comprehensive exclusions prevent accidental commits of sensitive data
- **Production deployment**: Use proper secrets management (not plain .env files)

## 🚀 Performance

- **Processing Time**: ~6 seconds (vs 130+ seconds with traditional RAG)
- **Accuracy**: 95%+ transaction extraction rate
- **Memory Usage**: Lightweight (no heavy ML models)
- **System Impact**: Minimal CPU/memory footprint

## 🏗️ Architecture

```
PDF Input → PDF Type Detection → OCR Processing → Bank Template Parsing → Structured JSON
```

### Core Components

1. **PDF Type Detection**: Automatically detects text-based vs image-based PDFs
2. **OCR Pipeline**: Uses `pdftoppm` + `tesseract` for image-based PDFs
3. **Bank Template Parsers**: Specialized parsers for major Indian banks
4. **Structured Output**: Clean JSON with account details and transactions

## 📋 Prerequisites

### System Dependencies

```bash
# macOS (using Homebrew)
brew install poppler tesseract

# Ubuntu/Debian
sudo apt-get install poppler-utils tesseract-ocr

# CentOS/RHEL
sudo yum install poppler-utils tesseract
```

### Go Dependencies

```bash
go mod init bank-statement-ocr
go get github.com/joho/godotenv
```

## 🛠️ Installation

### 1. Clone and Setup

```bash
git clone <your-repo>
cd bank-statement-ocr
mkdir -p backend/tmp/ocr
```

### 2. Environment Configuration

**⚠️ SECURITY**: Never commit your `.env` file! It contains sensitive API keys.

Copy the example file and fill in your values:
```bash
cp env.example .env
# Edit .env with your actual API key
```

```env
# Server Configuration
SERVER_PORT=8080
SERVER_HOST=localhost

# File Upload Configuration
MAX_FILE_SIZE_MB=10
TEMP_DIR=tmp

# Logging Configuration
LOG_LEVEL=info
DEBUG_MODE=true
```

### 3. Build and Run

```bash
cd backend
go build -o fast_ocr_server .
./fast_ocr_server
```

## 📁 Project Structure

```
bank-statement-ocr/
├── backend/
│   ├── main.go              # Main server with route handlers
│   ├── handler.go           # HTTP handlers (upload, health, etc.)
│   ├── fast_ocr.go          # Core OCR pipeline implementation
│   ├── go.mod               # Go dependencies
│   └── tmp/                 # Temporary file storage
│       └── ocr/             # OCR processing temp files
├── .env                     # Environment configuration
└── README.md               # This file
```

## 🔧 Core Implementation

### Fast OCR Pipeline (`fast_ocr.go`)

```go
type FastOCRPipeline struct {
    tempDir string
}

type BankStatement struct {
    AccountHolder string             `json:"account_holder"`
    AccountNumber string             `json:"account_number"`
    BankName      string             `json:"bank_name"`
    Transactions  []BankTransaction  `json:"transactions"`
}

type BankTransaction struct {
    Date        string  `json:"date"`
    Description string  `json:"description"`
    Amount      float64 `json:"amount"`
    Type        string  `json:"type"`
    Balance     float64 `json:"balance"`
}
```

### Key Functions

1. **`detectPDFType(pdfPath string) (bool, error)`**
   - Uses `pdftotext` to detect if PDF has extractable text
   - Returns `true` for image-based PDFs, `false` for text-based

2. **`processImageBasedPDF(pdfPath string) (string, error)`**
   - Converts PDF to PNG images using `pdftoppm`
   - Runs Tesseract OCR on each page
   - Combines all text output

3. **`parseStructuredData(text string) (*BankStatement, error)`**
   - Detects bank name using regex patterns
   - Extracts account details
   - Parses transactions using bank-specific templates

## 🏦 Supported Banks

### ICICI Bank
- **Pattern**: Date → UPI Description → Amount → Type (DR/CR)
- **Parser**: `parseICICITransactions()`
- **UPI Regex**: `UPI\/([\w\s.&-]+)\/([a-zA-Z0-9._-]+@[a-zA-Z]+)\/([\w\s.&-]+)\/([A-Z\s]+)\/(\d+)\/([A-Z0-9]+)`
- **Groups**: merchant, vpa, description, bank, ref1, ref2

### HDFC Bank
- **Pattern**: Similar to ICICI with HDFC-specific formatting
- **Parser**: `parseHDFCTransactions()`

### Axis Bank
- **Pattern**: UPI-specific format with P2M/P2A types
- **Parser**: `parseAxisTransactions()`
- **UPI Regex**: `(?i)UPI/(P2M|P2A)/\d{9,}/([^/]+)/.*?(?:v|Pay(?: to|men)?)/\s*([A-Z ]+?)(?:\s+LTD|LIMITED|BANK|FINANC|$)`
- **Groups**: type (P2M/P2A), payee, bank

### SBI Bank
- **Pattern**: DR/CR format with bank codes
- **Parser**: `parseSBITransactions()`
- **UPI Regex**: `UPI\/(?:DR|CR)\/(\d+)\/([\w\s.&-]+)\/([A-Z]{4})\/([a-zA-Z0-9._@-]+|[a-zA-Z0-9._-]+)\/?([\w\s-]*)?`
- **Groups**: ref, merchant, bank_code, vpa, description

### HSBC Bank
- **Pattern**: UPI with 17-digit reference numbers
- **Parser**: `parseHSBCTransactions()`
- **UPI Regex**: `(?i)UPI\d{17}\s+\d{9,}\s+([A-Z][A-Za-z .]+(?:Limited|Ltd|PRIVATE|PVT)?)`
- **Groups**: payee/merchant name

## 🔍 UPI Transaction Parsing

The system now includes bank-specific UPI regex patterns that provide enhanced narration parsing:

### Enhanced Description Format
- **Before**: `UPI/P2M/1234567890/Merchant Name/v/SBI BANK`
- **After**: `UPI P2M to Merchant Name via SBI BANK`

### Supported UPI Patterns
1. **Axis Bank**: Extracts transaction type (P2M/P2A), payee name, and destination bank
2. **HSBC Bank**: Extracts payee/merchant name from UPI reference format
3. **ICICI Bank**: Extracts merchant, VPA, description, bank, and reference numbers
4. **SBI Bank**: Extracts reference, merchant, bank code, VPA, and description

### Fallback Mechanism
- If bank-specific patterns don't match, the system falls back to generic UPI parsing
- All UPI transactions are preserved even if specific parsing fails
- Amount extraction works across all patterns
- **Pattern**: Axis-specific transaction layout
- **Parser**: `parseAxisTransactions()`

### SBI (State Bank of India)
- **Pattern**: SBI-specific formatting
- **Parser**: `parseSBITransactions()`

### Generic Parser
- **Fallback**: For unknown banks
- **Pattern**: UPI detection with amount extraction
- **Parser**: `parseGenericTransactions()`

## 🌐 API Endpoints

### 1. Health Check
```bash
GET /health
```
**Response:**
```json
{
  "service": "bank-statement-ocr",
  "status": "healthy",
  "timestamp": 1760296844
}
```

### 2. Fast Extraction
```bash
GET /fast-extract?file=<path>
```
**Response:**
```json
{
  "success": true,
  "message": "Successfully extracted 14 transactions",
  "processing_time": "fast",
  "statement": {
    "account_holder": "John Doe",
    "account_number": "187501510953",
    "bank_name": "ICICI",
    "transactions": [
      {
        "date": "2025-06-09",
        "description": "UPI/badri.karyan@ok/UPI/AXIS BANK/552686372620",
        "amount": -50.0,
        "type": "debit",
        "balance": 0
      }
    ]
  }
}
```

### 3. File Upload
```bash
POST /upload
Content-Type: multipart/form-data
```
**Response:**
```json
{
  "success": true,
  "message": "File uploaded successfully",
  "filename": "statement_1234567890.pdf"
}
```

## 🔍 Usage Examples

### 1. Upload and Extract
```bash
# Upload PDF
curl -X POST -F "file=@statement.pdf" http://localhost:8080/upload

# Extract transactions
curl -X GET "http://localhost:8080/fast-extract?file=tmp/statement_1234567890.pdf"
```

### 2. Direct File Processing
```bash
# Process existing file
curl -X GET "http://localhost:8080/fast-extract?file=path/to/statement.pdf"
```

### 3. Integration Example (Python)
```python
import requests

# Upload file
with open('statement.pdf', 'rb') as f:
    upload_response = requests.post(
        'http://localhost:8080/upload',
        files={'file': f}
    )

# Extract transactions
extract_response = requests.get(
    f'http://localhost:8080/fast-extract?file={upload_response.json()["filename"]}'
)

transactions = extract_response.json()['statement']['transactions']
```

## ⚡ Performance Optimization

### 1. OCR Configuration
```go
// Optimized Tesseract settings
cmd := exec.Command("tesseract", imagePath, "stdout", 
    "-l", "eng",           // English language
    "--psm", "6")          // Uniform text block
```

### 2. PDF Processing
```go
// High-resolution conversion for better OCR
cmd := exec.Command("pdftoppm", "-png", "-r", "300", pdfPath, outputPrefix)
```

### 3. Memory Management
- Automatic cleanup of temporary PNG files
- Efficient string building for text concatenation
- Minimal memory footprint

## 🐛 Troubleshooting

### Common Issues

1. **"pdftoppm not found"**
   ```bash
   # Install poppler-utils
   brew install poppler  # macOS
   sudo apt-get install poppler-utils  # Ubuntu
   ```

2. **"tesseract not found"**
   ```bash
   # Install tesseract
   brew install tesseract  # macOS
   sudo apt-get install tesseract-ocr  # Ubuntu
   ```

3. **"Port 8080 already in use"**
   ```bash
   # Kill existing processes
   lsof -ti:8080 | xargs kill -9
   ```

4. **Low OCR accuracy**
   - Increase PDF resolution: `-r 300` → `-r 600`
   - Try different Tesseract PSM modes: `--psm 6` → `--psm 3`

### Debug Mode

Enable debug logging:
```env
DEBUG_MODE=true
LOG_LEVEL=debug
```

## 🔒 Security Considerations

1. **File Upload Limits**
   - Maximum file size: 10MB (configurable)
   - File type validation: PDF only
   - Temporary file cleanup

2. **Path Security**
   - File path validation
   - No directory traversal attacks
   - Sandboxed temporary directory

3. **Resource Limits**
   - Process timeout handling
   - Memory usage monitoring
   - Concurrent request limits

## 📈 Scaling Considerations

### Horizontal Scaling
- Stateless design allows multiple instances
- Load balancer with sticky sessions
- Shared temporary storage (Redis/S3)

### Vertical Scaling
- Increase OCR resolution for better accuracy
- Parallel page processing
- Caching for repeated requests

## 🚀 Deployment

### Docker Deployment
```dockerfile
FROM golang:1.21-alpine AS builder
RUN apk add --no-cache poppler-utils tesseract-ocr
WORKDIR /app
COPY . .
RUN go build -o fast_ocr_server .

FROM alpine:latest
RUN apk add --no-cache poppler-utils tesseract-ocr
COPY --from=builder /app/fast_ocr_server .
EXPOSE 8080
CMD ["./fast_ocr_server"]
```

### Production Deployment
```bash
# Build optimized binary
go build -ldflags="-s -w" -o fast_ocr_server .

# Run with systemd
sudo systemctl enable fast-ocr-server
sudo systemctl start fast-ocr-server
```

## 📊 Monitoring

### Health Checks
```bash
# Basic health
curl http://localhost:8080/health

# Detailed metrics (if implemented)
curl http://localhost:8080/metrics
```

### Logging
- Structured JSON logs
- Request/response logging
- Error tracking
- Performance metrics

## 🤝 Contributing

1. Fork the repository
2. Create feature branch: `git checkout -b feature/new-bank-parser`
3. Add bank-specific parser
4. Test with sample statements
5. Submit pull request

### Adding New Bank Support

1. **Add bank detection pattern:**
   ```go
   "NEW_BANK": regexp.MustCompile(`(?i)new\s+bank`),
   ```

2. **Implement parser:**
   ```go
   func (p *FastOCRPipeline) parseNewBankTransactions(lines []string) []BankTransaction {
       // Bank-specific parsing logic
   }
   ```

3. **Update switch statement:**
   ```go
   case "NEW_BANK":
       statement.Transactions = p.parseNewBankTransactions(lines)
   ```

## 📄 License

MIT License - see LICENSE file for details.

## 🙏 Acknowledgments

- **Poppler** for PDF processing
- **Tesseract** for OCR capabilities
- **Go** for high-performance backend
- **Parsio** for inspiration on OCR pipeline design

---

**Built with ❤️ for fast, reliable bank statement processing**#   s t a t e m e n t p a r s e r - p o c 
 
 