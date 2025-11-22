// ABOUTME: Tests for monotonic precision formatting
// ABOUTME: Validates precision that never decreases across sequential calls

package main

import (
	"testing"
)

func TestFormatWithMonotonicPrecision(t *testing.T) {
	tests := []struct {
		name         string
		prev         float64
		curr         float64
		minPrecision int
		wantStr      string
		wantPrec     int
	}{
		{
			name:         "starts at min precision",
			prev:         1.0,
			curr:         2.0,
			minPrecision: 2,
			wantStr:      "2.00",
			wantPrec:     2,
		},
		{
			name:         "increases precision when needed",
			prev:         0.123,
			curr:         0.124,
			minPrecision: 2,
			wantStr:      "0.1240",
			wantPrec:     4,
		},
		{
			name:         "maintains minimum precision even when not needed",
			prev:         1.0,
			curr:         2.0,
			minPrecision: 5,
			wantStr:      "2.00000",
			wantPrec:     5,
		},
		{
			name:         "caps at max 10 decimals",
			prev:         0.12345678901,
			curr:         0.12345678902,
			minPrecision: 2,
			wantStr:      "0.1234567890",
			wantPrec:     10,
		},
		{
			name:         "enforces minimum of 2 decimals",
			prev:         1.0,
			curr:         2.0,
			minPrecision: 0,
			wantStr:      "2.00",
			wantPrec:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStr, gotPrec := FormatWithMonotonicPrecision(tt.prev, tt.curr, tt.minPrecision)
			if gotStr != tt.wantStr {
				t.Errorf("FormatWithMonotonicPrecision(%v, %v, %d) string = %q, want %q",
					tt.prev, tt.curr, tt.minPrecision, gotStr, tt.wantStr)
			}
			if gotPrec != tt.wantPrec {
				t.Errorf("FormatWithMonotonicPrecision(%v, %v, %d) precision = %d, want %d",
					tt.prev, tt.curr, tt.minPrecision, gotPrec, tt.wantPrec)
			}
		})
	}
}

// TestFormatWithMonotonicPrecision_Monotonic verifies precision never decreases
func TestFormatWithMonotonicPrecision_Monotonic(t *testing.T) {
	values := []float64{0.1, 0.11, 0.111, 0.1111, 0.5, 1.0, 0.9999}

	precision := 2
	for i := 1; i < len(values); i++ {
		prev := values[i-1]
		curr := values[i]

		_, newPrecision := FormatWithMonotonicPrecision(prev, curr, precision)

		if newPrecision < precision {
			t.Errorf("Precision decreased from %d to %d when formatting %v after %v",
				precision, newPrecision, curr, prev)
		}

		precision = newPrecision
	}
}
