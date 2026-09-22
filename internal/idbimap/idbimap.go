// Package idbimap converts IDBI Atlas wire types (internal/provider/idbi) into
// astra's own domain types. It is the single seam where IDBI's quirks are
// absorbed: string-typed numbers, {amountValue,currencyCode} money objects,
// the literal "NULL" for absent dates, "D"/"C" vs "DEBIT"/"CREDIT", and the
// three different statement shapes.
//
// Nothing above this package should import internal/provider/idbi or know an
// IDBI field name. Swap the provider (sandbox -> prod, or a mock) and these
// functions are all that needs re-checking.
package idbimap

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/yourusername/astra-backend/internal/provider/idbi"
)

// idbiTimeLayouts covers every timestamp/date format seen in the live capture.
var idbiTimeLayouts = []string{
	"2006-01-02T15:04:05.000Z",
	"2006-01-02T15:04:05.000",
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05",
	"2006-01-02",
	"02-01-2006",  // 442 customerLimits sanctionDate/expiryDate
	"02-Jan-2006", // 415 ckycGenDate
}

// ParseTime parses an IDBI timestamp. It returns the zero time (and no error)
// for absent values ("", "NULL", "null") so callers can treat "missing" and
// "unset" alike.
func ParseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	switch s {
	case "", "NULL", "null":
		return time.Time{}, nil
	}
	for _, l := range idbiTimeLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("idbimap: unrecognised time %q", s)
}

// MustParseTime is ParseTime with the error swallowed to the zero time — for
// display fields where a bad timestamp should not fail an ingest.
func MustParseTime(s string) time.Time {
	t, _ := ParseTime(s)
	return t
}

// ParseAmount parses an IDBI numeric string ("56780.25", "0", "", "NULL").
// Empty / "NULL" -> 0, no error.
func ParseAmount(s string) (float64, error) {
	s = strings.TrimSpace(s)
	switch s {
	case "", "NULL", "null":
		return 0, nil
	}
	s = strings.ReplaceAll(s, ",", "")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("idbimap: bad amount %q: %w", s, err)
	}
	return f, nil
}

// Money -> float64. Accepts the {amountValue,currencyCode} shape.
func Money(m idbi.Money) (float64, error) { return ParseAmount(m.AmountValue) }

// Amount -> float64. Accepts the {amount,currency} shape (used by 442).
func Amount(a idbi.Amount) (float64, error) { return ParseAmount(a.Amount) }

// MoneyF / AmountF are the error-swallowing variants for display paths.
func MoneyF(m idbi.Money) float64   { f, _ := Money(m); return f }
func AmountF(a idbi.Amount) float64 { f, _ := Amount(a); return f }

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}
