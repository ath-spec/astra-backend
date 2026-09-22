#!/bin/bash

# Bank Statement RAG System Test Script
# This script helps test the complete system

set -e

echo "🏦 Bank Statement RAG System Test"
echo "=================================="

# Check if GEMINI_API_KEY is set
if [ -z "$GEMINI_API_KEY" ]; then
    echo "❌ Error: GEMINI_API_KEY environment variable not set"
    echo "Please set it with: export GEMINI_API_KEY='your_key_here'"
    exit 1
fi

echo "✅ GEMINI_API_KEY is set"

# Check if Python dependencies are installed
echo "🔍 Checking Python dependencies..."
cd rag
if ! python3 -c "import google.generativeai, fitz, faiss, numpy" 2>/dev/null; then
    echo "📦 Installing Python dependencies..."
    pip install -r requirements.txt
else
    echo "✅ Python dependencies are installed"
fi

# Go back to project root
cd ..

# Check if Go is available
if ! command -v go &> /dev/null; then
    echo "❌ Error: Go is not installed or not in PATH"
    exit 1
fi

echo "✅ Go is available"

# Create a sample PDF text file for testing
echo "📄 Creating sample bank statement text..."
cat > sample_statement.txt << 'EOF'
BANK STATEMENT
Account Number: 1234567890
Statement Period: January 1, 2025 - January 31, 2025

Opening Balance: 25000.00

Date        Description                    Amount      Balance
2025-01-15  UPI Transfer to Flipkart      -1499.50    23500.50
2025-01-16  Salary Credit                  50000.00    73500.50
2025-01-17  ATM Withdrawal                 -2000.00    71500.50
2025-01-18  Online Shopping Amazon         -2500.00    69000.50
2025-01-19  Mobile Recharge                -500.00     68500.50
2025-01-20  Interest Credit                150.00     68650.50
2025-01-21  Restaurant Payment             -1200.00    67450.50
2025-01-22  Fuel Payment                   -800.00     66650.50
2025-01-23  Netflix Subscription          -999.00     65651.50
2025-01-24  Cash Deposit                   5000.00     70651.50

Closing Balance: 70651.50
EOF

echo "✅ Sample statement created"

# Test Python extraction directly
echo "🧪 Testing Python extraction..."
cd rag
python3 extract.py ../sample_statement.txt > ../test_output.json 2>/dev/null
cd ..

if [ -f test_output.json ] && [ -s test_output.json ]; then
    echo "✅ Python extraction test passed"
    echo "📊 Sample output:"
    head -20 test_output.json
else
    echo "❌ Python extraction test failed"
    exit 1
fi

# Start the Go server in background
echo "🚀 Starting Go server..."
cd backend
go run . &
SERVER_PID=$!
cd ..

# Wait for server to start
sleep 3

# Test health endpoint
echo "🏥 Testing health endpoint..."
if curl -s http://localhost:8080/health > /dev/null; then
    echo "✅ Health endpoint is working"
else
    echo "❌ Health endpoint test failed"
    kill $SERVER_PID 2>/dev/null || true
    exit 1
fi

# Test extraction endpoint
echo "🔍 Testing extraction endpoint..."
if curl -s "http://localhost:8080/extract?file=../sample_statement.txt" > api_output.json; then
    echo "✅ Extraction endpoint is working"
    echo "📊 API output:"
    head -20 api_output.json
else
    echo "❌ Extraction endpoint test failed"
fi

# Cleanup
echo "🧹 Cleaning up..."
kill $SERVER_PID 2>/dev/null || true
rm -f test_output.json api_output.json sample_statement.txt

echo ""
echo "🎉 All tests completed successfully!"
echo ""
echo "To run the system:"
echo "1. Set your API key: export GEMINI_API_KEY='your_key_here'"
echo "2. Start the server: cd backend && go run ."
echo "3. Test with: curl 'http://localhost:8080/extract?file=path/to/your.pdf'"
echo ""
echo "Happy banking! 🏦"
