// ABOUTME: Monotonic precision formatting for fitness values
// ABOUTME: Formats float64 pairs with precision that never decreases across calls

package main

import (
	"fmt"
	"math"
)

// FormatWithMonotonicPrecision formats curr with at least minPrecision decimals,
// or more if needed to distinguish from prev (up to max 10). Returns the formatted
// string and the precision actually used. Designed for monotonically increasing
// precision across multiple calls - pass the returned precision as minPrecision
// in the next call.
func FormatWithMonotonicPrecision(prev, curr float64, minPrecision int) (string, int) {
	const maxPrecision = 10

	// Handle special cases
	if math.IsNaN(prev) || math.IsNaN(curr) {
		return fmt.Sprintf("%.2f", curr), 2
	}
	if math.IsInf(prev, 0) || math.IsInf(curr, 0) {
		return fmt.Sprintf("%.2f", curr), 2
	}

	// Start with the minimum precision we've used so far
	neededPrecision := minPrecision
	if neededPrecision < 2 {
		neededPrecision = 2
	}

	// If they're exactly equal, use current minimum
	if prev == curr {
		if neededPrecision > maxPrecision {
			neededPrecision = maxPrecision
		}
		return fmt.Sprintf(fmt.Sprintf("%%.%df", neededPrecision), curr), neededPrecision
	}

	// Find the minimum precision where formatted strings differ
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

			// Use the maximum of clarity precision and our running minimum
			if clarityPrecision < neededPrecision {
				clarityPrecision = neededPrecision
			}

			return fmt.Sprintf(fmt.Sprintf("%%.%df", clarityPrecision), curr), clarityPrecision
		}
	}

	// Fallback to max precision if still can't distinguish
	return fmt.Sprintf(fmt.Sprintf("%%.%df", maxPrecision), curr), maxPrecision
}
