package service

import "regexp"

// PIIRedactor is the last-line-of-defence scrubber applied to any text that
// leaves the process for an external LLM. The live financial context is already
// assembled zero-PII by construction; this catches identifiers a user typed into
// the chat themselves ("my PAN is ABCDE1234F") before that text is forwarded or
// stored as memory.
type PIIRedactor struct {
	panRegex     *regexp.Regexp
	aadhaarRegex *regexp.Regexp
	acctRegex    *regexp.Regexp
	phoneRegex   *regexp.Regexp
}

func NewPIIRedactor() *PIIRedactor {
	return &PIIRedactor{
		// Indian PAN: 5 uppercase letters, 4 digits, 1 uppercase letter.
		panRegex: regexp.MustCompile(`\b[A-Z]{5}[0-9]{4}[A-Z]\b`),
		// Indian 10-digit mobile ([6-9] + 9 digits), optional +91 / 91 prefix.
		// The leading group (start-of-text or a non-digit) is captured and
		// re-emitted so a 10-digit run in the MIDDLE of a longer account number
		// can never be mistaken for a phone number. RE2 has no look-behind, so
		// this capture is how we assert "not preceded by a digit".
		phoneRegex: regexp.MustCompile(`(\A|[^\d])(?:\+?91[\s\-]?)?[6-9]\d{9}\b`),
		// 12-digit Aadhaar, optional single spaces between the 4-digit groups.
		aadhaarRegex: regexp.MustCompile(`\b\d{4}\s?\d{4}\s?\d{4}\b`),
		// Bank / demat account numbers: 12-18 digit runs. Runs after Aadhaar so
		// a bare 12-digit value is tagged Aadhaar; 13-18 digit runs land here.
		acctRegex: regexp.MustCompile(`\b\d{12,18}\b`),
	}
}

// Redact replaces sensitive identifiers with fixed placeholders. Order matters:
// PAN and phone (both structurally distinctive) run first, then Aadhaar claims
// bare 12-digit values, then the loose account-number sweep takes what is left.
func (r *PIIRedactor) Redact(text string) string {
	text = r.panRegex.ReplaceAllString(text, "[REDACTED_PAN]")
	text = r.phoneRegex.ReplaceAllString(text, "${1}[REDACTED_PHONE]")
	text = r.aadhaarRegex.ReplaceAllString(text, "[REDACTED_AADHAAR]")
	text = r.acctRegex.ReplaceAllString(text, "[REDACTED_ACCT]")
	return text
}
