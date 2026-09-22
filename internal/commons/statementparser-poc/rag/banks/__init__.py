"""
Bank-specific extraction modules
"""

import sys
from .base_bank import BaseBank
from .hdfc_bank import HDFCBank
from .icici_bank import ICICIBank
from .axis_bank import AxisBank
from .sbi_bank import SBIBank
from .hsbc_bank import HSBCBank
from .generic_bank import GenericBank

# Registry of all available banks
BANK_REGISTRY = [
    ICICIBank(debug_mode=True),  # Put ICICI first since it has specific patterns
    HDFCBank(debug_mode=True),
    AxisBank(debug_mode=True),
    SBIBank(debug_mode=True),
    HSBCBank(debug_mode=True),
    GenericBank(debug_mode=True),  # Should be last as fallback
]

def detect_bank(lines):
    """Detect which bank the statement is from"""
    for bank in BANK_REGISTRY:
        if bank.detect_bank(lines):
            return bank
    return BANK_REGISTRY[-1]  # Return generic bank as fallback
