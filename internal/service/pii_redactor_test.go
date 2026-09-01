package service

import "testing"

func TestPIIRedactor_Redact(t *testing.T) {
	redactor := NewPIIRedactor()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Redacts PAN Card",
			input:    "My PAN number is ABCDE1234F for verification",
			expected: "My PAN number is [REDACTED_PAN] for verification",
		},
		{
			name:     "Redacts 12-digit Aadhaar Number",
			input:    "Aadhaar: 1234 5678 9012 and 987654321098",
			expected: "Aadhaar: [REDACTED_AADHAAR] and [REDACTED_AADHAAR]",
		},
		{
			name:     "Redacts 12-18 digit Bank Account Numbers",
			input:    "Transfer to account 123456789012345 please",
			expected: "Transfer to account [REDACTED_ACCT] please",
		},
		{
			name:     "Redacts Indian Phone Numbers",
			input:    "Call me at +919876543210 or 9123456789",
			expected: "Call me at [REDACTED_PHONE] or [REDACTED_PHONE]",
		},
		{
			name:     "Preserves Non-Sensitive Currency and Dates",
			input:    "Invested ₹1,50,000 in Parag Parikh Flexi Cap Fund on 12 Jan 2026",
			expected: "Invested ₹1,50,000 in Parag Parikh Flexi Cap Fund on 12 Jan 2026",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactor.Redact(tt.input)
			if got != tt.expected {
				t.Errorf("Redact() = %q, want %q", got, tt.expected)
			}
		})
	}
}
