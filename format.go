// ABOUTME: Minimal precision formatting for fitness values
// ABOUTME: Formats float64 pairs with just enough digits to show the difference

package main

import (
	"fmt"
	"math"
)

// FormatMinimalPrecision returns a formatted string of curr with the minimum
// precision needed to distinguish it from prev. Returns a string suitable for
// displaying fitness values in CLI output.
func FormatMinimalPrecision(prev, curr float64) string {
	// Handle special cases
	if math.IsNaN(prev) || math.IsNaN(curr) {
		return fmt.Sprintf("%.2f", curr)
	}
	if math.IsInf(prev, 0) || math.IsInf(curr, 0) {
		return fmt.Sprintf("%.2f", curr)
	}

	// If they're exactly equal, use minimal precision
	if prev == curr {
		return fmt.Sprintf("%.2f", curr)
	}

	// Find the minimum precision where formatted strings differ
	const maxPrecision = 10
	for precision := 1; precision <= maxPrecision; precision++ {
		format := fmt.Sprintf("%%.%df", precision)
		prevStr := fmt.Sprintf(format, prev)
		currStr := fmt.Sprintf(format, curr)

		if prevStr != currStr {
			// Found differing precision, add 1 more digit for clarity
			clarityPrecision := precision + 1
			if clarityPrecision > maxPrecision {
				clarityPrecision = maxPrecision
			}
			return fmt.Sprintf(fmt.Sprintf("%%.%df", clarityPrecision), curr)
		}
	}

	// Fallback to max precision if still can't distinguish
	return fmt.Sprintf(fmt.Sprintf("%%.%df", maxPrecision), curr)
}
