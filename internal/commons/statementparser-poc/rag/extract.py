#!/usr/bin/env python3
"""
Simplified Bank Statement Extractor
Only handles bank detection and delegates to bank-specific files
"""

import fitz  # PyMuPDF
import sys
import json
from typing import List, Dict, Any

def extract_text_from_pdf(pdf_path: str) -> str:
    """Extract text from PDF using PyMuPDF"""
    try:
        doc = fitz.open(pdf_path)
        text = ""
        
        for page_num in range(len(doc)):
            page = doc[page_num]
            page_text = page.get_text("text")
            text += page_text + "\n"
        
        doc.close()
        return text
    except Exception as e:
        print(f"Error extracting text from PDF: {e}", file=sys.stderr)
        return ""

def extract_transactions(pdf_path: str) -> List[Dict[str, Any]]:
    """Main function to extract transactions from PDF"""
    try:
        print(f"🚀 Starting transaction extraction from: {pdf_path}", file=sys.stderr)
        
        # Extract text from PDF
        text = extract_text_from_pdf(pdf_path)
        if not text:
            print("❌ No text extracted from PDF", file=sys.stderr)
            return []
        
        print(f"📄 Extracted {len(text)} characters of text", file=sys.stderr)
        
        lines = text.split('\n')
        print(f"📝 Split into {len(lines)} lines", file=sys.stderr)
        
        # Detect the bank from the statement
        from banks import detect_bank
        # Banks are already initialized with debug_mode=True
        bank = detect_bank(lines)
        
        print(f"🏦 Detected bank: {bank.get_bank_name()}", file=sys.stderr)
        
        # Use bank-specific template for extraction
        transactions = bank.extract_transactions(lines)
        
        print(f"✅ {bank.get_bank_name().title()} template extracted {len(transactions)} transactions", file=sys.stderr)
        
        # Validate extracted transactions
        valid_transactions = []
        skipped_count = 0
        for i, txn in enumerate(transactions):
            if _validate_transaction(txn):
                valid_transactions.append(txn)
            else:
                skipped_count += 1
                print(f"⚠️ Skipping invalid transaction {i}: {txn.get('description', 'No description')[:50]}... (amount={txn.get('amount', 'N/A')})", file=sys.stderr)
        
        if skipped_count > 0:
            print(f"⚠️ Skipped {skipped_count} transactions due to validation", file=sys.stderr)
        
        print(f"✅ Validated {len(valid_transactions)} valid transactions out of {len(transactions)}", file=sys.stderr)
        
        return valid_transactions
        
    except Exception as e:
        print(f"❌ Error extracting transactions: {e}", file=sys.stderr)
        import traceback
        print(f"📄 Traceback: {traceback.format_exc()}", file=sys.stderr)
        return []

def _validate_transaction(transaction: Dict[str, Any]) -> bool:
    """Validate that a transaction has the required fields"""
    required_fields = ['description']
    
    for field in required_fields:
        if field not in transaction or transaction[field] is None:
            return False
    
    # Check if description is meaningful
    if not transaction['description'] or len(transaction['description'].strip()) < 3:
        return False
    
    # Check if amount is reasonable (only filter out extremely large amounts)
    if 'amount' in transaction and transaction['amount'] is not None:
        amount = abs(transaction['amount'])
        # Only filter out extremely large amounts (1 crore+)
        if amount > 10000000:  # More than 1 crore seems suspicious
            return False
    
    return True

def main():
    """Main function for command line usage"""
    if len(sys.argv) != 2:
        print("Usage: python extract.py <pdf_path>")
        sys.exit(1)
    
    pdf_path = sys.argv[1]
    transactions = extract_transactions(pdf_path)
    
    # Output as JSON with key per transaction (1-based index)
    by_key = {str(i + 1): txn for i, txn in enumerate(transactions)}
    print(json.dumps(by_key, indent=2))

if __name__ == "__main__":
    main()