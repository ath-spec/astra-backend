#!/bin/bash

# Test script for improved statementparser-poc
# This script tests the enhanced transaction extraction

echo "🧪 Testing Enhanced StatementParser-POC"
echo "======================================"

# Set up environment
export PYTHONPATH="/Users/swaraj/Documents/vault/z-backend/server/common/statementparser-poc/rag:$PYTHONPATH"
cd /Users/swaraj/Documents/vault/z-backend/server/common/statementparser-poc

# Test with a sample PDF (if available)
SAMPLE_PDF="test_samples/sample_bank_statement.pdf"

if [ -f "$SAMPLE_PDF" ]; then
    echo "📄 Testing with sample PDF: $SAMPLE_PDF"
    python rag/extract.py "$SAMPLE_PDF"
else
    echo "⚠️ No sample PDF found at $SAMPLE_PDF"
    echo "📝 Creating a test with mock data..."
    
    # Test the bank detection logic
    python3 -c "
import sys
sys.path.append('/Users/swaraj/Documents/vault/z-backend/server/common/statementparser-poc/rag')
from banks import detect_bank

# Test ICICI detection
icici_lines = [
    'ICICI BANK LIMITED',
    'DETAILED STATEMENT',
    'S No. Transaction Date Value Date Transaction Remarks Withdrawal Amt. Deposit Amt. Closing Balance',
    '1 01/01/24 01/01/24 UPI/SWIGGY@paytm 150.00 0.00 1000.00'
]
bank = detect_bank(icici_lines)
print(f'✅ ICICI Detection: {bank.get_bank_name()}')

# Test HDFC detection  
hdfc_lines = [
    'HDFC BANK LIMITED',
    'Withdrawal Amt. Deposit Amt. Closing Balance',
    '01/01/24 UPI payment to merchant 150.00 1000.00'
]
bank = detect_bank(hdfc_lines)
print(f'✅ HDFC Detection: {bank.get_bank_name()}')

# Test Generic fallback
generic_lines = [
    'Some Bank Statement',
    'Transaction details here'
]
bank = detect_bank(generic_lines)
print(f'✅ Generic Detection: {bank.get_bank_name()}')
"
fi

echo ""
echo "🔧 Testing Amount Extraction Patterns"
echo "===================================="

python3 -c "
import sys
sys.path.append('/Users/swaraj/Documents/vault/z-backend/server/common/statementparser-poc/rag')
from banks.generic_bank import GenericBank

bank = GenericBank(debug_mode=True)

# Test amount extraction
test_texts = [
    'UPI payment 1,234.56',
    'ATM withdrawal 500.00',
    'Salary credit 25000',
    'Transfer 1,500.75',
    'Bill payment 999.99'
]

for text in test_texts:
    amounts = bank.extract_amounts_from_text(text)
    tx_type, amount, withdrawal, deposit = bank.determine_transaction_type(text, amounts)
    print(f'Text: {text}')
    print(f'  Amounts: {amounts}')
    print(f'  Type: {tx_type}, Amount: {amount}, Withdrawal: {withdrawal}, Deposit: {deposit}')
    print()
"

echo ""
echo "✅ Enhanced StatementParser-POC Testing Complete"
echo "=============================================="
echo ""
echo "🎯 Key Improvements Made:"
echo "  • Enhanced amount extraction with multiple regex patterns"
echo "  • Improved transaction detection logic"
echo "  • Better bank-specific parsers (ICICI, HDFC)"
echo "  • Added generic fallback parser"
echo "  • Comprehensive debug logging"
echo "  • Transaction validation"
echo "  • Better error handling"
echo ""
echo "📊 Expected Results:"
echo "  • More accurate amount extraction"
echo "  • Better transaction detection"
echo "  • Reduced missed transactions"
echo "  • Improved categorization accuracy"
