package rmbff

import "testing"

func TestInrFull(t *testing.T) {
	cases := map[float64]string{
		0: "₹0", 999: "₹999", 1000: "₹1,000", 142000: "₹1,42,000",
		1234567: "₹12,34,567", -86000: "-₹86,000",
	}
	for in, want := range cases {
		if got := inrFull(in); got != want {
			t.Errorf("inrFull(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestInrCr(t *testing.T) {
	cases := map[float64]string{25000000: "₹2.5 Cr", 620000: "₹6.2 L", 4200: "₹4,200"}
	for in, want := range cases {
		if got := inrCr(in); got != want {
			t.Errorf("inrCr(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestDeltaPctNegativeBase(t *testing.T) {
	// Net cash flow going from -66k to -45k is an improvement (+), and from
	// -45k to -66k a deterioration (-); a naive (r-p)/p flips both signs.
	if d, ok := deltaPct(-45000, -66000); !ok || d <= 0 {
		t.Errorf("improving negative net: got %v ok=%v, want positive", d, ok)
	}
	if d, ok := deltaPct(-66000, -45000); !ok || d >= 0 {
		t.Errorf("worsening negative net: got %v ok=%v, want negative", d, ok)
	}
	if _, ok := deltaPct(10, 0); ok {
		t.Error("zero prior should report ok=false")
	}
}
