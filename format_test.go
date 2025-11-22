// ABOUTME: Tests for minimal precision formatting
// ABOUTME: Validates precision calculation for distinguishing float64 pairs

package main

import (
	"math"
	"testing"
)

func TestFormatMinimalPrecision(t *testing.T) {
	tests := []struct {
		name string
		prev float64
		curr float64
		want string
	}{
		// Identical numbers should use minimal precision
		{
			name: "identical numbers",
			prev: 1.5,
			curr: 1.5,
			want: "1.50",
		},
		{
			name: "identical zero",
			prev: 0.0,
			curr: 0.0,
			want: "0.00",
		},

		// Real CLI output examples from issue
		{
			name: "fitness values from CLI",
			prev: 0.1787662338,
			curr: 0.1756637807,
			want: "0.1757", // Differs at 3rd decimal (0.178 vs 0.175), show 4 for clarity
		},

		// Numbers differing at various decimal places
		{
			name: "differ at 1st decimal",
			prev: 1.1,
			curr: 1.2,
			want: "1.20", // Differs at 1st, show 2 for clarity
		},
		{
			name: "differ at 2nd decimal",
			prev: 1.11,
			curr: 1.12,
			want: "1.120", // Differs at 2nd, show 3 for clarity
		},
		{
			name: "differ at 5th decimal",
			prev: 0.123451,
			curr: 0.123459,
			want: "0.123459", // Differs at 5th, show 6 for clarity
		},
		{
			name: "differ at 8th decimal",
			prev: 0.12345678,
			curr: 0.12345679,
			want: "0.123456790", // Differs at 8th, show 9 for clarity
		},

		// Very small differences
		{
			name: "very small difference",
			prev: 1.0000000001,
			curr: 1.0000000002,
			want: "1.0000000002", // Max precision
		},

		// Edge cases
		{
			name: "negative numbers",
			prev: -1.123,
			curr: -1.124,
			want: "-1.1240",
		},
		{
			name: "zero vs small number",
			prev: 0.0,
			curr: 0.001,
			want: "0.0010", // Differs at 3rd decimal, show 4 for clarity
		},
		{
			name: "large numbers",
			prev: 12345.67,
			curr: 12345.68,
			want: "12345.680",
		},

		// Special float values
		{
			name: "NaN",
			prev: 0.0,
			curr: math.NaN(),
			want: "NaN",
		},
		{
			name: "infinity",
			prev: 0.0,
			curr: math.Inf(1),
			want: "+Inf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatMinimalPrecision(tt.prev, tt.curr)
			if got != tt.want {
				t.Errorf("FormatMinimalPrecision(%v, %v) = %q, want %q",
					tt.prev, tt.curr, got, tt.want)
			}
		})
	}
}

// TestFormatMinimalPrecision_Symmetric verifies that order doesn't matter
// for determining precision (though the returned value is always curr)
func TestFormatMinimalPrecision_Symmetric(t *testing.T) {
	a, b := 0.123, 0.124

	// Both should use same precision (differ at 3rd decimal, show 4)
	resultAB := FormatMinimalPrecision(a, b)
	resultBA := FormatMinimalPrecision(b, a)

	// Extract precision by counting decimal places
	countDecimals := func(s string) int {
		for i := len(s) - 1; i >= 0; i-- {
			if s[i] == '.' {
				return len(s) - i - 1
			}
		}
		return 0
	}

	precisionAB := countDecimals(resultAB)
	precisionBA := countDecimals(resultBA)

	if precisionAB != precisionBA {
		t.Errorf("Precision should be symmetric: FormatMinimalPrecision(%v, %v) has %d decimals, but FormatMinimalPrecision(%v, %v) has %d decimals",
			a, b, precisionAB, b, a, precisionBA)
	}
}
