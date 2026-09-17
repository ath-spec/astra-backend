"""
HDFC Bank specific extraction logic
"""

import re
import sys
from typing import List, Dict, Any
from .base_bank import BaseBank

class HDFCBank(BaseBank):
    """HDFC Bank specific transaction extraction"""
    
    def __init__(self, debug_mode: bool = False):
        super().__init__(debug_mode)
        self.debug_mode = debug_mode
    
    def detect_bank(self, lines: List[str]) -> bool:
        """Detect if this is an HDFC Bank statement"""
        # Check first 50 lines for the actual bank name
        sample_text = ' '.join(lines[:50]).upper()
        
        # First check for explicit HDFC bank name
        bank_name_patterns = [
            'HDFC BANK LIMITED',
            'HDFC BANK LTD',
            'HDFC BANK'
        ]
        
        for pattern in bank_name_patterns:
            if pattern in sample_text:
                return True
        
        # Check for HDFC-specific format patterns
        # HDFC statements have specific column headers
        if 'WITHDRAWAL AMT.' in sample_text and 'DEPOSIT AMT.' in sample_text and 'CLOSING BALANCE' in sample_text:
            return True
        
        return False
    
    def extract_transactions(self, lines: List[str]) -> List[Dict[str, Any]]:
        """Extract transactions from HDFC Bank statements - SPECIFIC FORMAT"""
        transactions = []
        
        self.debug_print("🏦 HDFC: Using specific HDFC format extractor")
        
        # Look for the header line
        header_line = -1
        for i, line in enumerate(lines[:10]):
            self.debug_print(f"🔍 HDFC: Checking line {i}: {line[:100]}...")
            # Check if this line contains withdrawal and deposit headers
            if 'Withdrawal Amt.' in line and 'Deposit Amt.' in line:
                header_line = i
                break
            # Also check for individual headers and look at next lines
            elif 'Withdrawal Amt.' in line:
                # Check if next lines contain the other headers
                for j in range(i+1, min(i+3, len(lines))):
                    if 'Deposit Amt.' in lines[j]:
                        # Check if Closing Balance is in the same line or next line
                        if 'Closing Balance' in lines[j] or (j+1 < len(lines) and 'Closing Balance' in lines[j+1]):
                            header_line = i
                            break
                if header_line != -1:
                    break
        
        if header_line == -1:
            self.debug_print("⚠️ HDFC: Header not found, using fallback")
            return self._extract_multiline_transactions(lines)
        
        self.debug_print(f"🔍 HDFC: Found header at line {header_line}")
        
        # Process transactions starting after header
        # HDFC format: Each transaction spans 8 lines:
        # Line 0: Date (transaction date)
        # Line 1-2: Description parts
        # Line 3: Transaction ID
        # Line 4: Date (value date)
        # Line 5: Amount (withdrawal or deposit)
        # Line 6: Balance
        # Line 7: Reference
        
        i = header_line + 1
        
        # Skip the header continuation lines (Deposit Amt., Closing Balance)
        while i < len(lines) and i < header_line + 5:
            if re.match(r'\d{2}/\d{2}/\d{2}', lines[i].strip()):
                break
            i += 1
        
        self.debug_print(f"🔍 HDFC: Starting transaction processing from line {i}")
        
        while i < len(lines):
            line = lines[i].strip()
            
            # Look for date pattern (DD/MM/YY) - this is the transaction date
            if re.match(r'\d{2}/\d{2}/\d{2}', line):
                date = line
                self.debug_print(f"🔍 HDFC: Found transaction starting at line {i}: {date}")
                
                # Collect description (next 2-3 lines until we hit transaction ID or another date)
                description_parts = []
                j = i + 1
                
                while j < len(lines) and j < i + 10:
                    next_line = lines[j].strip()
                    
                    # Stop if we hit a date or transaction ID (pure digits or specific patterns)
                    if re.match(r'\d{2}/\d{2}/\d{2}', next_line):
                        break
                    if re.match(r'^\d{10,}$', next_line):  # Transaction ID (10+ digits)
                        j += 1  # Skip the transaction ID
                        break
                    
                    # Collect description parts
                    if next_line and not re.match(r'^\d+$', next_line):
                        description_parts.append(next_line)
                    
                    j += 1
                
                # Now j should be pointing to the line after transaction ID
                # Look for the value date (should be a date)
                if j < len(lines) and re.match(r'\d{2}/\d{2}/\d{2}', lines[j].strip()):
                    j += 1  # Skip the value date
                
                # Now look for amounts (withdrawal/deposit and balance) with improved patterns
                amounts_found = []
                for k in range(j, min(j + 8, len(lines))):
                    amount_line = lines[k].strip()
                    self.debug_print(f"🔍 HDFC: Checking amount line {k}: {amount_line[:100]}...")
                    
                    # Try multiple amount patterns
                    amount_patterns = [
                        r'(\d{1,3}(?:,\d{3})*\.\d{2})',   # 1,234.56
                        r'(\d+\.\d{2})',                   # 123.45
                        r'(\d+)',                          # 123
                        r'^\d{1,3}(?:,\d{3})*\.\d{2}$',   # Exact match
                        r'^\d+\.\d{2}$',                   # Exact decimal
                        r'^\d+$'                           # Exact integer
                    ]
                    
                    for pattern in amount_patterns:
                        amount_match = re.search(pattern, amount_line)
                        if amount_match:
                            amount_str = amount_match.group(1) if amount_match.groups() else amount_match.group(0)
                            amount_str = amount_str.replace(',', '')
                            try:
                                amount = float(amount_str)
                                if amount > 0:  # Only positive amounts
                                    amounts_found.append(amount)
                                    self.debug_print(f"🔍 HDFC: Found amount {amount} at line {k} with pattern {pattern}")
                                    break
                            except ValueError:
                                continue
                
                self.debug_print(f"🔍 HDFC: Found {len(amounts_found)} amounts: {amounts_found}")
                
                # Process the transaction if we have amounts or description
                if len(amounts_found) >= 1 or description_parts:
                    if description_parts:
                        description = ' '.join(description_parts)
                    else:
                        # If no description found, use a generic one
                        description = "Transaction"
                    
                    # Initialize amounts
                    withdrawal = 0.0
                    deposit = 0.0
                    balance = 0.0
                    
                    if len(amounts_found) >= 1:
                        # HDFC format: first amount is withdrawal/deposit, second is balance (if available)
                        first_amount = amounts_found[0]
                        balance = amounts_found[1] if len(amounts_found) >= 2 else 0.0
                        
                        # Determine if this is a withdrawal or deposit based on context
                        # Look for keywords in description
                        description_lower = description.lower()
                        
                        if any(keyword in description_lower for keyword in ['upi', 'atm', 'debit', 'withdrawal', 'transfer', 'pos', 'card']):
                            # This is likely a withdrawal
                            withdrawal = first_amount
                            deposit = 0.0
                            amount = -withdrawal  # Negative for withdrawal
                            transaction_type = "debit"
                        elif any(keyword in description_lower for keyword in ['credit', 'deposit', 'neft', 'imps', 'salary', 'refund']):
                            # This is likely a deposit
                            withdrawal = 0.0
                            deposit = first_amount
                            amount = deposit  # Positive for deposit
                            transaction_type = "credit"
                        else:
                            # Default to withdrawal if unclear
                            withdrawal = first_amount
                            deposit = 0.0
                            amount = -withdrawal  # Negative for withdrawal
                            transaction_type = "debit"
                    else:
                        # No amounts found, but we have description - create a placeholder transaction
                        amount = 0.0
                        transaction_type = "debit"
                    
                    transaction = {
                        "date": date,
                        "description": description,
                        "withdrawal": withdrawal,
                        "deposit": deposit,
                        "amount": amount,
                        "balance": balance,
                        "transaction_type": transaction_type,
                        "bank": "hdfc",
                        "parsed_with": "hdfc_specific"
                    }
                    
                    transactions.append(transaction)
                    
                    self.debug_print(f"✅ HDFC: {description[:40]}... -> Amount: {amount}, Balance: {balance}")
                    
                    # Move to the next transaction
                    # Start from the current line and look for the next date
                    i = i + 1  # Move to next line after current transaction date
                    while i < len(lines) and not re.match(r'\d{2}/\d{2}/\d{2}', lines[i].strip()):
                        i += 1
                else:
                    i += 1
            else:
                i += 1
        
        self.debug_print(f"🔍 HDFC: Extracted {len(transactions)} transactions")
        
        return transactions
    
    def _extract_multiline_transactions(self, lines: List[str]) -> List[Dict[str, Any]]:
        """Fallback method for multi-line format"""
        transactions = []
        
        # Find the header line
        header_line = -1
        for i, line in enumerate(lines):
            if 'Withdrawal Amt.' in line and 'Deposit Amt.' in line:
                header_line = i
                break
        
        if header_line == -1:
            return transactions
        
        # Process transactions starting after headers
        i = header_line + 1
        while i < len(lines):
            # Look for date pattern (DD/MM/YY)
            if re.match(r'\d{2}/\d{2}/\d{2}', lines[i].strip()):
                date = lines[i].strip()
                
                # Collect description lines (next few lines until we hit amounts)
                description_parts = []
                j = i + 1
                
                # Look for UPI or transaction description
                while j < len(lines) and j < i + 10:  # Limit search range
                    line = lines[j].strip()
                    
                    # Stop if we hit a date or amount
                    if re.match(r'\d{2}/\d{2}/\d{2}', line) or re.search(r'\d{1,3}(?:,\d{3})*\.\d{2}', line):
                        break
                    
                    # Collect description parts
                    if line and not re.match(r'^\d+$', line):  # Skip pure numbers
                        description_parts.append(line)
                    
                    j += 1
                
                # Look for amounts in the next few lines
                withdrawal = 0.0
                deposit = 0.0
                balance = 0.0
                
                # Search for amounts after description
                for k in range(j, len(lines)):
                    line = lines[k].strip()
                    amount_match = re.search(r'(\d{1,3}(?:,\d{3})*\.\d{2})', line)
                    if amount_match:
                        amount_str = amount_match.group(1).replace(',', '')
                        amount = float(amount_str)
                        
                        # Determine if this is withdrawal, deposit, or balance
                        # Based on position and context
                        if k == j:  # First amount is usually withdrawal
                            withdrawal = amount
                        elif k == j + 1:  # Second amount is usually balance
                            balance = amount
                        else:
                            # Could be deposit or additional amount
                            if amount > withdrawal:  # If larger than withdrawal, likely balance
                                balance = amount
                            else:
                                deposit = amount
                
                # Create transaction if we found a description
                if description_parts:
                    description = ' '.join(description_parts)
                    
                    # Determine transaction type and amount
                    if withdrawal > 0:
                        amount = -withdrawal  # Negative for withdrawal
                        transaction_type = "debit"
                    elif deposit > 0:
                        amount = deposit  # Positive for deposit
                        transaction_type = "credit"
                    else:
                        amount = 0.0
                        transaction_type = "debit"  # Default
                    
                    transaction = {
                        "date": date,
                        "description": description,
                        "withdrawal": withdrawal,
                        "deposit": deposit,
                        "amount": amount,
                        "balance": balance,
                        "transaction_type": transaction_type,
                        "bank": "hdfc",
                        "parsed_with": "hdfc_multiline_fallback"
                    }
                    
                    transactions.append(transaction)
                
                # Move to next potential transaction
                i = j + 1
            else:
                i += 1
        
        return transactions
