#!/usr/bin/env python3
import json
import sys

# Simple test script that always succeeds
transactions = [
    {
        "date": "2025-01-15",
        "description": "Test Transaction",
        "amount": -100.0,
        "balance": 1000.0,
        "transaction_type": "debit"
    }
]

print(json.dumps(transactions, indent=2))
sys.exit(0)
