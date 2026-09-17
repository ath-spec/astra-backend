"""
SBI Bank specific extraction logic
"""

import sys
import re
from typing import List, Dict, Any
from .base_bank import BaseBank

class SBIBank(BaseBank):
    """SBI Bank specific transaction extraction"""
    
    def detect_bank(self, lines: List[str]) -> bool:
        """Detect if this is an SBI Bank statement"""
        sample_text = ' '.join(lines[:200]).upper()
        
        sbi_patterns = [
            'STATE BANK OF INDIA',
            'SBI BANK',
            'SBI',
            'STATE BANK'
        ]
        
        for pattern in sbi_patterns:
            if pattern in sample_text:
                return True
        
        return False
    
    def extract_transactions(self, lines: List[str]) -> List[Dict[str, Any]]:
        """Extract transactions from SBI Bank statements - PROFESSIONAL VERSION"""
        transactions = []
        
        self.debug_print("🏦 SBI: Using professional SBI extractor")
        
        try:
            # Process ALL lines - no artificial limits
            max_lines = len(lines)
            
            # Enhanced patterns for better matching
            date_pattern = re.compile(r'\d{1,2}\s+\w{3}(?:\s+\d{4})?')  # Handle both "14 Sep" and "14 Sep 2025"
            amount_pattern = re.compile(r'\d{1,3}(?:,\d{3})*\.\d{2}')
            
            # Find transaction start point
            start_line = -1
            for i in range(min(200, len(lines))):
                if 'Txn Date' in lines[i] or 'Transaction Date' in lines[i]:
                    start_line = i + 10  # Skip header lines
                    break
            
            if start_line == -1:
                # Fallback: look for first date pattern
                for i in range(min(200, len(lines))):
                    if date_pattern.match(lines[i].strip()):
                        start_line = i
                        break
            
            if start_line == -1:
                self.debug_print("⚠️ SBI: No transaction start found")
                return transactions
            
            self.debug_print(f"🔍 SBI: Starting from line {start_line}, processing ALL {max_lines} lines")
            
            # Process transactions with improved logic - NO LIMITS
            i = start_line
            transaction_count = 0
            processed_dates = set()
            
            while i < max_lines:  # Process ALL lines
                line = lines[i].strip()
                
                # Look for date pattern
                if date_pattern.match(line):
                    transaction_count += 1
                    
                    self.debug_print(f"🔍 SBI: Processing transaction {transaction_count}: {line}")
                    
                    # Extract transaction details
                    txn_date = line
                    description_parts = []
                    amounts_found = []
                    
                    # Look ahead for description and amounts
                    j = i + 1
                    while j < min(i + 12, max_lines):
                        if j >= len(lines):
                            break
                        
                        next_line = lines[j].strip()
                        
                        # Stop if we hit another date
                        if date_pattern.match(next_line):
                            break
                        
                        # Skip empty lines
                        if not next_line:
                            j += 1
                            continue
                        
                        # Check for amount
                        if amount_pattern.match(next_line):
                            amount = float(next_line.replace(',', ''))
                            amounts_found.append(amount)
                            self.debug_print(f"🔍 SBI: Found amount {amount} at line {j}")
                        # Check for description (not just numbers or years)
                        elif not re.match(r'^\d+$', next_line) and not re.match(r'^\d{4}$', next_line):
                            # Clean description - remove unwanted text
                            clean_line = next_line
                            
                            # Remove common unwanted patterns
                            unwanted_patterns = [
                                r'Please do not share.*',
                                r'ATM:.*',
                                r'OTP:.*',
                                r'PIN:.*',
                                r'MICR:.*',
                                r'Bank never asks.*',
                                r'Automated Teller.*',
                                r'One Time Password.*',
                                r'Personal Identification.*',
                                r'Magnetic Ink.*'
                            ]
                            
                            for pattern in unwanted_patterns:
                                clean_line = re.sub(pattern, '', clean_line, flags=re.IGNORECASE)
                            
                            clean_line = clean_line.strip()
                            if clean_line and len(clean_line) > 2:
                                description_parts.append(clean_line)
                        
                        j += 1
                    
                    # Create transaction if we have amounts
                    if amounts_found:
                        # Build clean description
                        if description_parts:
                            description = ' '.join(description_parts)
                            # Limit description length
                            if len(description) > 100:
                                description = description[:100] + "..."
                        else:
                            description = "SBI Transaction"
                        
                        # Determine transaction type and amounts
                        withdrawal = 0.0
                        deposit = 0.0
                        balance = 0.0
                        amount = 0.0
                        transaction_type = "debit"
                        
                        # Get balance if available
                        if len(amounts_found) >= 2:
                            balance = amounts_found[1]
                        
                        # Determine transaction type based on description
                        description_lower = description.lower()
                        
                        if any(keyword in description_lower for keyword in [
                            'to transfer', 'upi', 'atm', 'debit', 'withdrawal', 'transfer to',
                            'sent', 'payment', 'purchase', 'withdraw'
                        ]):
                            withdrawal = amounts_found[0]
                            amount = -withdrawal
                            transaction_type = "debit"
                        elif any(keyword in description_lower for keyword in [
                            'by transfer', 'credit', 'deposit', 'salary', 'received',
                            'transfer from', 'refund', 'cashback'
                        ]):
                            deposit = amounts_found[0]
                            amount = deposit
                            transaction_type = "credit"
                        else:
                            # Default to withdrawal for unknown types
                            withdrawal = amounts_found[0]
                            amount = -withdrawal
                            transaction_type = "debit"
                        
                        transaction = {
                            "date": txn_date,
                            "description": description,
                            "withdrawal": withdrawal,
                            "deposit": deposit,
                            "amount": amount,
                            "balance": balance,
                            "transaction_type": transaction_type,
                            "bank": "sbi",
                            "parsed_with": "sbi_professional"
                        }
                        
                        transactions.append(transaction)
                        self.debug_print(f"✅ SBI: Transaction {transaction_count}: {description[:50]}... -> Amount: {amount}")
                    
                    # Move to next potential transaction
                    i = j
                else:
                    i += 1
            
            self.debug_print(f"🔍 SBI: Successfully extracted {len(transactions)} transactions")
            
        except Exception as e:
            self.debug_print(f"⚠️ SBI: Error during extraction: {str(e)}")
            transactions = [{
                "date": "Error",
                "description": "SBI extraction error",
                "withdrawal": 0.0,
                "deposit": 0.0,
                "amount": 0.0,
                "balance": 0.0,
                "transaction_type": "debit",
                "bank": "sbi",
                "parsed_with": "sbi_error"
            }]
        
        return transactions
