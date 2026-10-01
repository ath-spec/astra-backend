package rmbff

import (
	"fmt"
	"math"
	"strings"
)

const crore = 1e7

// inrCr formats a rupee amount as "₹12.4 Cr" (or lakhs / plain rupees when small).
func inrCr(v float64) string {
	switch {
	case math.Abs(v) >= crore:
		return fmt.Sprintf("₹%.1f Cr", v/crore)
	case math.Abs(v) >= 1e5:
		return fmt.Sprintf("₹%.1f L", v/1e5)
	default:
		return inrFull(v)
	}
}

func toCr(v float64) float64 { return math.Round(v/crore*100) / 100 }

func pctOf(part, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return math.Round(part/total*1000) / 10
}

func firstName(full string) string {
	if f := strings.Fields(full); len(f) > 0 {
		return f[0]
	}
	return full
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// inrFull formats a rupee amount with Indian digit grouping: ₹1,42,000.
func inrFull(v float64) string {
	n := int64(math.Round(math.Abs(v)))
	s := fmt.Sprint(n)
	if len(s) > 3 {
		head, tail := s[:len(s)-3], s[len(s)-3:]
		var parts []string
		for len(head) > 2 {
			parts = append([]string{head[len(head)-2:]}, parts...)
			head = head[:len(head)-2]
		}
		if head != "" {
			parts = append([]string{head}, parts...)
		}
		s = strings.Join(parts, ",") + "," + tail
	}
	if v < 0 {
		return "-₹" + s
	}
	return "₹" + s
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func initials(name string) string {
	var out []rune
	for _, w := range strings.Fields(name) {
		if len(out) == 2 {
			break
		}
		out = append(out, []rune(strings.ToUpper(w))[0])
	}
	return string(out)
}

func upper(s string) string { return strings.ToUpper(s) }
