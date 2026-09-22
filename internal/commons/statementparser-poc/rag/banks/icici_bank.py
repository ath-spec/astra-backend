"""
ICICI Bank specific extraction logic
"""

import re
import sys
from typing import List, Dict, Any
from .base_bank import BaseBank

class ICICIBank(BaseBank):
    """ICICI Bank specific transaction extraction"""
    
    def detect_bank(self, lines: List[str]) -> bool:
        """Detect if this is an ICICI Bank statement"""
        # Check first 50 lines for the actual bank name
        sample_text = ' '.join(lines[:50]).upper()
        
        # First check for explicit ICICI bank name
        bank_name_patterns = [
            'ICICI BANK LIMITED',
            'ICICI BANK LTD',
            'ICICI BANK'
        ]
        
        for pattern in bank_name_patterns:
            if pattern in sample_text:
                return True
        
        # Document issuer often appears in footer: prefer issuer over UPI text
        full_text = ' '.join(lines).upper()
        if 'TEAM ICICI BANK' in full_text or ('STATEMENT OF TRANSACTIONS' in full_text and 'ICICI' in full_text):
            return True
        
        # Check for ICICI-specific format patterns
        # ICICI statements often start with "DETAILED STATEMENT"
        if 'DETAILED STATEMENT' in sample_text:
            # Additional check for ICICI-specific column headers
            if 'S NO.' in sample_text and 'TRANSACTION REMARKS' in sample_text:
                return True
        
        return False
    
    def extract_transactions(self, lines: List[str]) -> List[Dict[str, Any]]:
        """Extract transactions from ICICI Bank statement format (robust version)"""
        transactions = []

        self.debug_print("🏦 ICICI: Using improved ICICI-specific extractor")

        # --- Find header line ---
        header_line = -1
        for i, line in enumerate(lines[:50]):
            if 'S No.' in line:
                for j in range(i + 1, min(i + 10, len(lines))):
                    l = lines[j]
                    if 'Value Date' in l or 'Transaction Date' in l or 'Transaction Remarks' in l:
                        header_line = i
                        break
                    # "Transaction" and "Date" on consecutive lines (e.g. column headers)
                    if j + 1 < len(lines) and 'Transaction' in l and 'Date' in lines[j + 1]:
                        header_line = i
                        break
                if header_line != -1:
                    break

        if header_line == -1:
            self.debug_print("⚠️ ICICI: Header not found")
            return transactions

        self.debug_print(f"🔍 ICICI: Found header at line {header_line}")

        # --- Skip header continuation lines ---
        i = header_line + 1
        while i < len(lines) and i < header_line + 15:
            if re.match(r'^\d+\b', lines[i].strip()):
                break
            i += 1

        self.debug_print(f"🔍 ICICI: Starting transaction processing from line {i}")

        # --- Transaction extraction loop ---
        while i < len(lines):
            line = lines[i].strip()

            # Match lines starting with ONLY a transaction number (1-3 digits, followed by whitespace or end of line)
            # Support both DD/MM/YYYY and DD.MM.YYYY date formats on next line
            next_has_date = (i + 1 < len(lines) and
                            re.match(r'\d{2}[./]\d{2}[./]\d{4}', lines[i + 1].strip()))
            match = re.match(r'^(\d{1,3})\s+|\n', line) or (re.match(r'^\d{1,3}$', line) and next_has_date)
            
            if match:
                transaction_num = match.group(1) if match.groups() else re.match(r'\d{1,3}', line).group(0)
                self.debug_print(f"🔍 ICICI: Found transaction {transaction_num} at line {i}")

                # Collect all lines for this transaction block dynamically
                current_data_lines = [line]
                i += 1
                consecutive_empty = 0
                max_empty_lines = 2  # Allow 2 consecutive empty lines before stopping
                seen_amount = False  # Track if we've seen amounts
                lines_since_amount = 0
                
                while i < len(lines):
                    next_line = lines[i].strip()
                    
                    # Stop if next line starts with another transaction number
                    next_is_transaction = (re.match(r'^\d{1,3}\s+', next_line) or 
                                         re.match(r'^\d{1,3}$', next_line))
                    
                    if next_is_transaction:
                        break
                    
                    # Track empty lines
                    if not next_line:
                        consecutive_empty += 1
                        if consecutive_empty > max_empty_lines:
                            # Too many empty lines, probably end of transaction
                            break
                    else:
                        consecutive_empty = 0
                        
                        # Check if this looks like an amount
                        if re.match(r'^\d+\.\d{2}$', next_line) or re.match(r'^\d{1,3}(?:,\d{3})*\.\d{2}$', next_line):
                            seen_amount = True
                            lines_since_amount = 0
                        
                        # If we've seen amounts, stop collecting after a few more lines
                        if seen_amount:
                            lines_since_amount += 1
                            if lines_since_amount > 3:  # Stop 3 lines after last amount
                                break
                    
                    current_data_lines.append(next_line)
                    i += 1

                # Combine transaction text block
                block = " ".join(current_data_lines)
                parts = block.split()

                # --- Extract key fields (support DD/MM/YYYY and DD.MM.YYYY) ---
                dates = re.findall(r'\d{2}[./]\d{2}[./]\d{4}', block)
                # Normalize to DD/MM/YYYY for consistency
                dates = [d.replace('.', '/') for d in dates]
                value_date = dates[0] if len(dates) > 0 else ""
                transaction_date = dates[1] if len(dates) > 1 else value_date

                # --- Extract amounts (exclude date parts like 19.01 from 19.01.2026) ---
                block_for_amounts = re.sub(r'\d{2}[./]\d{2}[./]\d{4}', ' ', block)
                amounts = re.findall(r'\d+\.\d{2}', block_for_amounts)
                withdrawal = deposit = balance = 0.0
                if len(amounts) >= 3:
                    withdrawal, deposit, balance = map(float, amounts[:3])
                elif len(amounts) == 2:
                    # Two amounts: (txn amount, balance) - infer debit/credit from description
                    txn_amt, balance = float(amounts[0]), float(amounts[1])
                    debit_keywords = ('UPI', 'BIL', 'VSI', 'MMT', 'IMPS', 'INFT', 'NEFT', 'RCHG', 'DTAX', 'BPAY', 'IDTX', 'BBPS', 'PAVC', 'ATM', 'POS')
                    desc_upper = block.upper()
                    if any(kw in desc_upper for kw in debit_keywords):
                        withdrawal, deposit = txn_amt, 0.0
                    else:
                        withdrawal, deposit = 0.0, txn_amt
                elif len(amounts) == 1:
                    withdrawal = float(amounts[0])

                # --- Extract description ---
                # Remove dates (slash and dot) and amounts to isolate description text
                desc = re.sub(r'\d{2}[./]\d{2}[./]\d{4}', '', block)
                desc = re.sub(r'\d+\.\d{2}', '', desc)
                desc = re.sub(r'^\d+\s*', '', desc).strip()

                # Determine transaction type and signed amount
                if withdrawal > 0 and deposit == 0:
                    amount = -withdrawal
                    transaction_type = "debit"
                elif deposit > 0:
                    amount = deposit
                    transaction_type = "credit"
                else:
                    amount = 0.0
                    transaction_type = "debit"

                transaction = {
                    "date": transaction_date,
                    "description": desc,
                    "withdrawal": withdrawal,
                    "deposit": deposit,
                    "amount": amount,
                    "balance": balance,
                    "transaction_type": transaction_type,
                    "bank": "icici",
                    "parsed_with": "icici_specific",
                }

                
                transactions.append(transaction)
                self.debug_print(f"✅ ICICI: {desc[:50]}... -> Amount: {amount}, Balance: {balance}")

            else:
                # No match, move on
                i += 1

        self.debug_print(f"🔍 ICICI: Extracted {len(transactions)} transactions")
        
        # Remove duplicates (same date, description, amount, balance, and type)
        seen = set()
        unique_txns = []
        for t in transactions:
            key = (
                t.get('date'),
                t.get('description'),
                round(t.get('amount', 0.0), 2),
                round(t.get('balance', 0.0), 2),
                t.get('transaction_type')
            )
            if key not in seen:
                seen.add(key)
                unique_txns.append(t)
        
        self.debug_print(f"🔍 ICICI: After deduplication: {len(unique_txns)} transactions (removed {len(transactions) - len(unique_txns)} duplicates)")
        
        return unique_txns
