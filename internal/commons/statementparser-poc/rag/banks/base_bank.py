"""
Base bank class for all bank-specific extraction logic
"""

from abc import ABC, abstractmethod
from typing import List, Dict, Any, Optional
import re
import sys

class BaseBank(ABC):
    """Base class for all bank-specific extraction logic"""
    
    def __init__(self, debug_mode: bool = False):
        self.debug_mode = debug_mode
    
    @abstractmethod
    def detect_bank(self, lines: List[str]) -> bool:
        """Detect if this bank's format matches the statement"""
        pass
    
    @abstractmethod
    def extract_transactions(self, lines: List[str]) -> List[Dict[str, Any]]:
        """Extract transactions using bank-specific logic"""
        pass
    
    def get_bank_name(self) -> str:
        """Return bank name"""
        return self.__class__.__name__.lower().replace('bank', '')
    
    def debug_print(self, message: str):
        """Print debug message if debug mode is enabled"""
        if self.debug_mode:
            print(message, file=sys.stderr)
    
    def clean_and_decode_text(self, text: str) -> str:
        """Clean and decode OCR-corrupted text to make it more readable"""
        # Remove excessive whitespace and normalize
        text = re.sub(r'\s+', ' ', text.strip())
        
        # Fix common OCR errors - be more selective
        replacements = {
            # Fix common OCR character substitutions for UPI
            'l\'PI': 'UPI',
            'L\'Pl': 'UPI', 
            'L\'PI': 'UPI',
            'l\'Pl': 'UPI',
            
            # Remove only the most problematic special characters
            '\\': '',
            '"': '',
            "'": '',
            '~': '',
            '•': '',
            '·': '',
            '■': '',
            '◄': '',
            '°': '',
            '→': '',
            '←': '',
            '↑': '',
            '↓': '',
            '↔': '',
            '↕': '',
            '↖': '',
            '↗': '',
            '↘': '',
            '↙': '',
            '↩': '',
            '↪': '',
            '↫': '',
            '↬': '',
            '↭': '',
            '↮': '',
            '↯': '',
            '↰': '',
            '↱': '',
            '↲': '',
            '↳': '',
            '↴': '',
            '↵': '',
            '↶': '',
            '↷': '',
            '↸': '',
            '↹': '',
            '↺': '',
            '↻': '',
            '↼': '',
            '↽': '',
            '↾': '',
            '↿': '',
            '⇀': '',
            '⇁': '',
            '⇂': '',
            '⇃': '',
            '⇄': '',
            '⇅': '',
            '⇆': '',
            '⇇': '',
            '⇈': '',
            '⇉': '',
            '⇊': '',
            '⇋': '',
            '⇌': '',
            '⇍': '',
            '⇎': '',
            '⇏': '',
            '⇐': '',
            '⇑': '',
            '⇒': '',
            '⇓': '',
            '⇔': '',
            '⇕': '',
            '⇖': '',
            '⇗': '',
            '⇘': '',
            '⇙': '',
            '⇚': '',
            '⇛': '',
            '⇜': '',
            '⇝': '',
            '⇞': '',
            '⇟': '',
            '⇠': '',
            '⇡': '',
            '⇢': '',
            '⇣': '',
            '⇤': '',
            '⇥': '',
            '⇦': '',
            '⇧': '',
            '⇨': '',
            '⇩': '',
            '⇪': '',
            '⇫': '',
            '⇬': '',
            '⇭': '',
            '⇮': '',
            '⇯': '',
            '⇰': '',
            '⇱': '',
            '⇲': '',
            '⇳': '',
            '⇴': '',
            '⇵': '',
            '⇶': '',
            '⇷': '',
            '⇸': '',
            '⇹': '',
            '⇺': '',
            '⇻': '',
            '⇼': '',
            '⇽': '',
            '⇾': '',
            '⇿': '',
        }
        
        # Apply replacements
        for old, new in replacements.items():
            text = text.replace(old, new)
        
        # Clean up multiple spaces
        text = re.sub(r'\s+', ' ', text)
        
        return text.strip()
    
    def extract_amounts_from_text(self, text: str) -> List[float]:
        """Extract all amounts from text using multiple patterns"""
        amounts = []
        
        # Multiple amount patterns in order of specificity
        amount_patterns = [
            r'(\d{1,3}(?:,\d{3})*\.\d{2})',   # 1,234.56
            r'(\d+\.\d{2})',                   # 123.45
            r'(\d{4,})',                       # 1234 (4+ digits)
            r'(\d+)',                          # Any number
        ]
        
        for pattern in amount_patterns:
            matches = re.findall(pattern, text)
            for match in matches:
                try:
                    amount_str = match.replace(',', '')
                    amount = float(amount_str)
                    if amount > 0 and amount not in amounts:  # Avoid duplicates
                        amounts.append(amount)
                except ValueError:
                    continue
        
        return sorted(amounts, reverse=True)  # Return largest amounts first
    
    def determine_transaction_type(self, description: str, amounts: List[float]) -> tuple:
        """Determine transaction type and amount based on description and amounts"""
        desc_lower = description.lower()
        
        # Keywords that indicate debit/withdrawal
        debit_keywords = [
            'upi', 'atm', 'debit', 'withdrawal', 'transfer', 'pos', 'card',
            'purchase', 'payment', 'bill', 'charge', 'fee', 'penalty'
        ]
        
        # Keywords that indicate credit/deposit
        credit_keywords = [
            'credit', 'deposit', 'neft', 'imps', 'salary', 'refund',
            'cashback', 'reward', 'interest', 'dividend', 'bonus'
        ]
        
        # Check for debit indicators
        is_debit = any(keyword in desc_lower for keyword in debit_keywords)
        is_credit = any(keyword in desc_lower for keyword in credit_keywords)
        
        if amounts:
            amount = amounts[0]
            if is_debit and not is_credit:
                return "debit", -amount, amount, 0.0
            elif is_credit and not is_debit:
                return "credit", amount, 0.0, amount
            else:
                # Ambiguous - default to debit for positive amounts
                return "debit", -amount, amount, 0.0
        else:
            return "debit", 0.0, 0.0, 0.0
    
    def find_transaction_boundaries(self, lines: List[str], start_idx: int) -> tuple:
        """Find the start and end of a transaction block"""
        # Look for common transaction start patterns
        start_patterns = [
            r'^\d+$',                    # Transaction number
            r'\d{2}/\d{2}/\d{2}',      # Date pattern
            r'\d{1,2}/\d{1,2}/\d{4}',  # Full date pattern
        ]
        
        # Look for common transaction end patterns
        end_patterns = [
            r'^\d{1,3}(?:,\d{3})*\.\d{2}$',  # Amount at end of line
            r'^\d+\.\d{2}$',                   # Decimal amount
            r'^\d+$',                          # Integer amount
        ]
        
        # Find transaction start
        transaction_start = start_idx
        for i in range(start_idx, min(start_idx + 20, len(lines))):
            line = lines[i].strip()
            for pattern in start_patterns:
                if re.match(pattern, line):
                    transaction_start = i
                    break
            if transaction_start != start_idx:
                break
        
        # Find transaction end
        transaction_end = min(transaction_start + 15, len(lines))
        for i in range(transaction_start, min(transaction_start + 15, len(lines))):
            line = lines[i].strip()
            for pattern in end_patterns:
                if re.match(pattern, line):
                    transaction_end = i + 1
                    break
        
        return transaction_start, transaction_end