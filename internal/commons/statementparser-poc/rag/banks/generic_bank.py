"""
Generic bank parser for unknown or unsupported bank formats
"""

import re
import sys
from typing import List, Dict, Any
from .base_bank import BaseBank

class GenericBank(BaseBank):
    """Generic bank parser that can handle most bank statement formats"""
    
    def detect_bank(self, lines: List[str]) -> bool:
        """Generic bank always returns True as fallback"""
        return True
    
    def extract_transactions(self, lines: List[str]) -> List[Dict[str, Any]]:
        """Extract transactions using generic patterns"""
        transactions = []
        
        self.debug_print("🏦 GENERIC: Using generic transaction extractor")
        
        # Look for transaction patterns across the entire document
        i = 0
        while i < len(lines):
            line = lines[i].strip()
            
            # Look for potential transaction indicators
            if self._is_transaction_line(line):
                self.debug_print(f"🔍 GENERIC: Found potential transaction at line {i}: {line[:50]}...")
                
                # Extract transaction data from surrounding lines
                transaction = self._extract_transaction_block(lines, i)
                if transaction:
                    transactions.append(transaction)
                    self.debug_print(f"✅ GENERIC: Extracted transaction: {transaction['description'][:30]}... -> {transaction['amount']}")
                
                # Skip ahead to avoid duplicate processing
                i += transaction.get('lines_processed', 5)
            else:
                i += 1
        
        self.debug_print(f"🔍 GENERIC: Extracted {len(transactions)} transactions")
        return transactions
    
    def _is_transaction_line(self, line: str) -> bool:
        """Check if a line looks like it contains transaction data"""
        if not line or len(line) < 5:
            return False
        
        # Look for common transaction patterns
        transaction_patterns = [
            r'\d{2}/\d{2}/\d{2}',           # Date pattern
            r'\d{1,2}/\d{1,2}/\d{4}',      # Full date pattern
            r'upi',                          # UPI transaction
            r'atm',                          # ATM transaction
            r'neft',                         # NEFT transaction
            r'imps',                         # IMPS transaction
            r'rtgs',                         # RTGS transaction
            r'\d{1,3}(?:,\d{3})*\.\d{2}',  # Amount pattern
            r'\d+\.\d{2}',                   # Decimal amount
        ]
        
        line_lower = line.lower()
        for pattern in transaction_patterns:
            if re.search(pattern, line_lower):
                return True
        
        return False
    
    def _extract_transaction_block(self, lines: List[str], start_idx: int) -> Dict[str, Any]:
        """Extract a complete transaction from a block of lines"""
        transaction = {
            "date": "",
            "description": "",
            "amount": 0.0,
            "transaction_type": "debit",
            "withdrawal": 0.0,
            "deposit": 0.0,
            "balance": 0.0,
            "bank": "generic",
            "parsed_with": "generic_fallback",
            "lines_processed": 1
        }
        
        # Collect lines for this transaction (up to 10 lines)
        block_lines = []
        for i in range(start_idx, min(start_idx + 10, len(lines))):
            line = lines[i].strip()
            if line:
                block_lines.append(line)
            else:
                break
        
        if not block_lines:
            return None
        
        # Join all lines for analysis
        full_text = ' '.join(block_lines)
        transaction["lines_processed"] = len(block_lines)
        
        # Extract date
        date_patterns = [
            r'(\d{2}/\d{2}/\d{2})',         # DD/MM/YY
            r'(\d{1,2}/\d{1,2}/\d{4})',     # DD/MM/YYYY
            r'(\d{2}-\d{2}-\d{2})',         # DD-MM-YY
            r'(\d{1,2}-\d{1,2}-\d{4})',     # DD-MM-YYYY
        ]
        
        for pattern in date_patterns:
            match = re.search(pattern, full_text)
            if match:
                transaction["date"] = match.group(1)
                break
        
        # Extract amounts
        amounts = self.extract_amounts_from_text(full_text)
        
        # Extract description (remove dates and amounts)
        description = full_text
        
        # Remove date patterns
        for pattern in date_patterns:
            description = re.sub(pattern, '', description)
        
        # Remove amount patterns
        amount_patterns = [
            r'\d{1,3}(?:,\d{3})*\.\d{2}',
            r'\d+\.\d{2}',
            r'\b\d{4,}\b',  # Large numbers (likely amounts)
        ]
        
        for pattern in amount_patterns:
            description = re.sub(pattern, '', description)
        
        # Clean up description
        description = re.sub(r'\s+', ' ', description).strip()
        if not description:
            description = "Transaction"
        
        transaction["description"] = description
        
        # Determine transaction type and amount
        if amounts:
            tx_type, amount, withdrawal, deposit = self.determine_transaction_type(description, amounts)
            transaction["transaction_type"] = tx_type
            transaction["amount"] = amount
            transaction["withdrawal"] = withdrawal
            transaction["deposit"] = deposit
            
            # Use second amount as balance if available
            if len(amounts) > 1:
                transaction["balance"] = amounts[1]
        else:
            # No amounts found, create placeholder
            transaction["amount"] = 0.0
            transaction["transaction_type"] = "debit"
        
        return transaction