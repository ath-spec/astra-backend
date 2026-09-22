#!/usr/bin/env python3
"""
Test script for bank-specific UPI regex patterns
"""

import re
import sys
import os

# Add the rag directory to the path
sys.path.append(os.path.join(os.path.dirname(__file__), 'rag'))

from extract import detect_bank_type, parse_upi_with_bank_pattern, extract_amount_from_context

def test_bank_detection():
    """Test bank detection functionality"""
    print("Testing bank detection...")
    
    test_cases = [
        (["AXIS BANK LIMITED", "Account Statement"], "axis"),
        (["HSBC BANK", "Statement"], "hsbc"),
        (["ICICI BANK", "Account Details"], "icici"),
        (["STATE BANK OF INDIA", "Statement"], "sbi"),
        (["UNKNOWN BANK", "Statement"], "unknown"),
    ]
    
    for lines, expected in test_cases:
        result = detect_bank_type(lines)
        status = "✅" if result == expected else "❌"
        print(f"{status} {lines[0]} -> {result} (expected: {expected})")

def test_upi_patterns():
    """Test UPI regex patterns with sample data"""
    print("\nTesting UPI regex patterns...")
    
    # Bank-specific UPI regex patterns
    bank_patterns = {
        'axis': {
            'pattern': r'(?i)UPI/(P2M|P2A)/\d{9,}/([^/]+)/.*?(?:v|Pay(?: to|men)?)/\s*([A-Z ]+?)(?:\s+LTD|LIMITED|BANK|FINANC|$)',
            'groups': ['type', 'payee', 'bank'],
            'test_cases': [
                "UPI/P2M/1234567890/Merchant Name/v/SBI BANK",
                "UPI/P2A/9876543210/John Doe/Pay to/AXIS BANK",
            ]
        },
        'hsbc': {
            'pattern': r'(?i)UPI\d{17}\s+\d{9,}\s+([A-Z][A-Za-z .]+(?:Limited|Ltd|PRIVATE|PVT)?)',
            'groups': ['payee'],
            'test_cases': [
                "UPI12345678901234567 1234567890 AMAZON LIMITED",
                "UPI98765432109876543 9876543210 SWIGGY PVT",
            ]
        },
        'icici': {
            'pattern': r'UPI\/([\w\s.&-]+)\/([a-zA-Z0-9._-]+@[a-zA-Z]+)\/([\w\s.&-]+)\/([A-Z\s]+)\/(\d+)\/([A-Z0-9]+)',
            'groups': ['merchant', 'vpa', 'description', 'bank', 'ref1', 'ref2'],
            'test_cases': [
                "UPI/ZOMATO LIM/zomato-order@p/Zomato Pay/YES BANK L/292365793762/PTM509278035975",
            ]
        },
        'sbi': {
            'pattern': r'UPI\/(?:DR|CR)\/(\d+)\/([\w\s.&-]+)\/([A-Z]{4})\/([a-zA-Z0-9._@-]+|[a-zA-Z0-9._-]+)\/?([\w\s-]*)?',
            'groups': ['ref', 'merchant', 'bank_code', 'vpa', 'description'],
            'test_cases': [
                "TO TRANSFER-UPI/DR/527597124519/Swiggy Ltd/UTIB/swiggyupi@/Pay-",
            ]
        }
    }
    
    for bank_name, config in bank_patterns.items():
        print(f"\n--- Testing {bank_name.upper()} patterns ---")
        pattern = re.compile(config['pattern'])
        
        for test_case in config['test_cases']:
            match = pattern.search(test_case)
            if match:
                print(f"✅ Match found: {test_case[:50]}...")
                for i, group in enumerate(config['groups'], 1):
                    if i <= len(match.groups()):
                        print(f"   {group}: {match.group(i)}")
            else:
                print(f"❌ No match: {test_case[:50]}...")

def test_amount_extraction():
    """Test amount extraction functionality"""
    print("\nTesting amount extraction...")
    
    test_cases = [
        ("UPI/P2M/1234567890/Merchant/Amount: 1500.00", 1500.0),
        ("UPI payment 2500.50 to merchant", 2500.5),
        ("UPI transaction without amount", 0.0),
        ("UPI/P2A/9876543210/John/Amount: 750.25", 750.25),
    ]
    
    for line, expected in test_cases:
        result = extract_amount_from_context(line)
        status = "✅" if abs(result - expected) < 0.01 else "❌"
        print(f"{status} '{line[:30]}...' -> {result} (expected: {expected})")

def main():
    """Run all tests"""
    print("🧪 Testing UPI Regex Patterns Integration")
    print("=" * 50)
    
    test_bank_detection()
    test_upi_patterns()
    test_amount_extraction()
    
    print("\n" + "=" * 50)
    print("✅ All tests completed!")

if __name__ == "__main__":
    main()
