package payments

import (
	"testing"
	"time"

	paymentsdomain "github.com/yourusername/astra-backend/internal/domain/payments"
)

func TestNextDebitDate(t *testing.T) {
	from := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		frequency string
		want      time.Time
	}{
		{paymentsdomain.FrequencyMonthly, time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)}, // Jan 31 + 1 month normalizes past Feb
		{paymentsdomain.FrequencyQuarterly, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)},
		{paymentsdomain.FrequencyYearly, time.Date(2027, 1, 31, 0, 0, 0, 0, time.UTC)},
		{"", time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)}, // unrecognized falls back to monthly
	}

	for _, tt := range tests {
		got := nextDebitDate(from, tt.frequency)
		if !got.Equal(tt.want) {
			t.Errorf("nextDebitDate(%v, %q) = %v, want %v", from, tt.frequency, got, tt.want)
		}
	}
}

func TestNextDebitDate_MidMonthMonthlyStaysSameDay(t *testing.T) {
	from := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	got := nextDebitDate(from, paymentsdomain.FrequencyMonthly)
	want := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("nextDebitDate(%v, MONTHLY) = %v, want %v", from, got, want)
	}
}
