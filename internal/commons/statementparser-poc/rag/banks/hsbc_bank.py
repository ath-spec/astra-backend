"""
HSBC Bank specific extraction logic
"""

import sys
from typing import List, Dict, Any
from .base_bank import BaseBank

class HSBCBank(BaseBank):
    """HSBC Bank specific transaction extraction"""
    
    def detect_bank(self, lines: List[str]) -> bool:
        """Detect if this is an HSBC Bank statement"""
        sample_text = ' '.join(lines[:200]).upper()
        
        hsbc_patterns = [
            'HSBC BANK LIMITED',
            'HSBC BANK LTD',
            'HSBC BANK',
            'HSBC'
        ]
        
        for pattern in hsbc_patterns:
            if pattern in sample_text:
                return True
        
        return False
    
    def extract_transactions(self, lines: List[str]) -> List[Dict[str, Any]]:
        """Extract transactions from HSBC Bank statements"""
        self.debug_print("🏦 HSBC: Using HSBC-specific extractor")
        # TODO: Implement HSBC-specific extraction logic
        # For now, return empty list
        return []
