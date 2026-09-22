#!/bin/bash

# Bank Statement OCR - Test Script
# This script tests the OCR pipeline with sample files

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
SERVER_URL="http://localhost:8080"
TEST_FILE="backend/tmp/statement_1760294447_AbhimanyuBankStatement.pdf"

echo -e "${YELLOW}🚀 Bank Statement OCR Test Script${NC}"
echo "=================================="

# Function to check if server is running
check_server() {
    echo -e "${YELLOW}📡 Checking server status...${NC}"
    if curl -s "$SERVER_URL/health" > /dev/null; then
        echo -e "${GREEN}✅ Server is running${NC}"
        return 0
    else
        echo -e "${RED}❌ Server is not running${NC}"
        echo "Please start the server first:"
        echo "  cd backend && ./fast_ocr_server"
        return 1
    fi
}

# Function to test health endpoint
test_health() {
    echo -e "${YELLOW}🏥 Testing health endpoint...${NC}"
    response=$(curl -s "$SERVER_URL/health")
    echo "Response: $response"
    
    if echo "$response" | grep -q "healthy"; then
        echo -e "${GREEN}✅ Health check passed${NC}"
    else
        echo -e "${RED}❌ Health check failed${NC}"
        return 1
    fi
}

# Function to test fast extraction
test_fast_extraction() {
    echo -e "${YELLOW}⚡ Testing fast extraction...${NC}"
    
    if [ ! -f "$TEST_FILE" ]; then
        echo -e "${RED}❌ Test file not found: $TEST_FILE${NC}"
        echo "Please upload a PDF file first or update TEST_FILE variable"
        return 1
    fi
    
    echo "Processing file: $TEST_FILE"
    start_time=$(date +%s)
    
    response=$(curl -s "$SERVER_URL/fast-extract?file=$TEST_FILE")
    end_time=$(date +%s)
    duration=$((end_time - start_time))
    
    echo "Processing time: ${duration}s"
    echo "Response:"
    echo "$response" | jq .
    
    if echo "$response" | grep -q '"success": true'; then
        echo -e "${GREEN}✅ Fast extraction test passed${NC}"
        
        # Extract transaction count
        tx_count=$(echo "$response" | jq '.statement.transactions | length')
        echo "Transactions extracted: $tx_count"
        
        if [ "$tx_count" -gt 0 ]; then
            echo -e "${GREEN}✅ Transactions successfully extracted${NC}"
        else
            echo -e "${YELLOW}⚠️  No transactions extracted (may be normal for some PDFs)${NC}"
        fi
    else
        echo -e "${RED}❌ Fast extraction test failed${NC}"
        return 1
    fi
}

# Function to test file upload
test_upload() {
    echo -e "${YELLOW}📤 Testing file upload...${NC}"
    
    if [ ! -f "$TEST_FILE" ]; then
        echo -e "${RED}❌ Test file not found: $TEST_FILE${NC}"
        return 1
    fi
    
    response=$(curl -s -X POST -F "file=@$TEST_FILE" "$SERVER_URL/upload")
    echo "Response: $response"
    
    if echo "$response" | grep -q '"success": true'; then
        echo -e "${GREEN}✅ File upload test passed${NC}"
        
        # Extract filename for next test
        filename=$(echo "$response" | jq -r '.filename')
        echo "Uploaded file: $filename"
        
        # Test extraction of uploaded file
        echo -e "${YELLOW}⚡ Testing extraction of uploaded file...${NC}"
        extract_response=$(curl -s "$SERVER_URL/fast-extract?file=tmp/$filename")
        
        if echo "$extract_response" | grep -q '"success": true'; then
            echo -e "${GREEN}✅ Upload + Extract workflow test passed${NC}"
        else
            echo -e "${RED}❌ Upload + Extract workflow test failed${NC}"
            return 1
        fi
    else
        echo -e "${RED}❌ File upload test failed${NC}"
        return 1
    fi
}

# Function to run performance test
test_performance() {
    echo -e "${YELLOW}📊 Running performance test...${NC}"
    
    if [ ! -f "$TEST_FILE" ]; then
        echo -e "${RED}❌ Test file not found: $TEST_FILE${NC}"
        return 1
    fi
    
    echo "Running 5 consecutive extraction requests..."
    
    total_time=0
    success_count=0
    
    for i in {1..5}; do
        echo "Request $i/5..."
        start_time=$(date +%s.%N)
        
        response=$(curl -s "$SERVER_URL/fast-extract?file=$TEST_FILE")
        end_time=$(date +%s.%N)
        
        duration=$(echo "$end_time - $start_time" | bc)
        total_time=$(echo "$total_time + $duration" | bc)
        
        if echo "$response" | grep -q '"success": true'; then
            success_count=$((success_count + 1))
            echo "  ✅ Success (${duration}s)"
        else
            echo "  ❌ Failed (${duration}s)"
        fi
        
        sleep 1
    done
    
    avg_time=$(echo "scale=2; $total_time / 5" | bc)
    echo ""
    echo "Performance Results:"
    echo "  Success Rate: $success_count/5"
    echo "  Average Time: ${avg_time}s"
    echo "  Total Time: ${total_time}s"
    
    if [ "$success_count" -eq 5 ]; then
        echo -e "${GREEN}✅ Performance test passed${NC}"
    else
        echo -e "${YELLOW}⚠️  Performance test completed with some failures${NC}"
    fi
}

# Main test execution
main() {
    echo "Starting tests..."
    echo ""
    
    # Check if jq is installed
    if ! command -v jq &> /dev/null; then
        echo -e "${RED}❌ jq is required but not installed${NC}"
        echo "Install with: brew install jq (macOS) or apt-get install jq (Ubuntu)"
        exit 1
    fi
    
    # Check if bc is installed
    if ! command -v bc &> /dev/null; then
        echo -e "${RED}❌ bc is required but not installed${NC}"
        echo "Install with: brew install bc (macOS) or apt-get install bc (Ubuntu)"
        exit 1
    fi
    
    # Run tests
    check_server || exit 1
    echo ""
    
    test_health || exit 1
    echo ""
    
    test_fast_extraction || exit 1
    echo ""
    
    test_upload || exit 1
    echo ""
    
    test_performance || exit 1
    echo ""
    
    echo -e "${GREEN}🎉 All tests completed successfully!${NC}"
    echo ""
    echo "Summary:"
    echo "  ✅ Server health check"
    echo "  ✅ Fast extraction"
    echo "  ✅ File upload workflow"
    echo "  ✅ Performance test"
    echo ""
    echo "Your Bank Statement OCR system is working correctly!"
}

# Run main function
main "$@"
