// ABOUTME: Provides Camelot wheel harmonic mixing utilities
// ABOUTME: Functions for parsing keys and calculating harmonic compatibility between tracks

package playlist

import (
	"fmt"
	"regexp"
	"strconv"
)

// CamelotKey represents a parsed Camelot key
type CamelotKey struct {
	Letter byte // 'A' (minor) or 'B' (major)
	Number int  // 1-12
}

// Compile regex once at package initialization
var camelotKeyRegex = regexp.MustCompile(`^(\d+)([AB])$`)

// Harmonic distance constants representing DJ mixing compatibility
const (
	harmonicPerfect       = 0  // Perfect match: same key
	harmonicExcellent     = 1  // Excellent: relative major/minor or ±1 number same letter
	harmonicDramatic      = 2  // Dramatic: parallel major/minor (mood shift)
	harmonicIncompatible  = 10 // Incompatible: all other transitions
)

// ParseCamelotKey parses a Camelot key string like "8A" into structured form
// Returns error if the key format is invalid
func ParseCamelotKey(key string) (*CamelotKey, error) {
	if key == "" {
		return nil, fmt.Errorf("empty key")
	}

	matches := camelotKeyRegex.FindStringSubmatch(key)
	if len(matches) != 3 {
		return nil, fmt.Errorf("invalid key format: %s", key)
	}

	number, err := strconv.Atoi(matches[1])
	if err != nil || number < 1 || number > 12 {
		return nil, fmt.Errorf("invalid key number: %s", matches[1])
	}

	return &CamelotKey{
		Letter: matches[2][0], // Take first byte of "A" or "B"
		Number: number,
	}, nil
}

// String returns the string representation of a CamelotKey
func (k *CamelotKey) String() string {
	return fmt.Sprintf("%d%c", k.Number, k.Letter)
}

// Compare returns -1 if k < other, 0 if k == other, 1 if k > other
// Sorts by letter first (A before B), then by number (1-12)
// Nil keys are sorted last
func (k *CamelotKey) Compare(other *CamelotKey) int {
	// Handle nil cases
	if k == nil && other == nil {
		return 0
	}
	if k == nil {
		return 1 // nil sorts last
	}
	if other == nil {
		return -1 // nil sorts last
	}

	// Sort by letter first (A before B)
	if k.Letter != other.Letter {
		return int(k.Letter - other.Letter)
	}

	// Then by number
	return k.Number - other.Number
}

// IsParallelMajorMinor detects if two keys are parallel major/minor (same root note, different mode)
// For example: C Major (8B) ↔ C Minor (5A), F Major (7B) ↔ F Minor (4A)
// This represents a dramatic mood shift according to harmonic mixing theory
// The Camelot wheel pattern: xA (minor) ↔ (x+3)B (major) with wraparound
func IsParallelMajorMinor(k1, k2 *CamelotKey) bool {
	if k1 == nil || k2 == nil {
		return false
	}

	// Keys must have different letters (one A, one B)
	if k1.Letter == k2.Letter {
		return false
	}

	// Check if k1 is A (minor) and k2 is the parallel B (major)
	if k1.Letter == 'A' {
		parallelMajor := (k1.Number+2)%12 + 1
		if k2.Number == parallelMajor {
			return true
		}
	}

	// Check if k1 is B (major) and k2 is the parallel A (minor)
	if k1.Letter == 'B' {
		parallelMinor := (k1.Number+8)%12 + 1 // Equivalent to (k1.Number - 3 + 12) % 12 + 1
		if k2.Number == parallelMinor {
			return true
		}
	}

	return false
}

// HarmonicDistanceParsed calculates harmonic compatibility using pre-parsed keys
// This is much faster than HarmonicDistance as it skips parsing
// Returns 10 if either key is nil (same as other bad transitions)
func HarmonicDistanceParsed(k1, k2 *CamelotKey) int {
	// If either key is invalid, treat as bad transition
	if k1 == nil || k2 == nil {
		return harmonicIncompatible
	}

	// Same key = perfect match
	if k1.Number == k2.Number && k1.Letter == k2.Letter {
		return harmonicPerfect
	}

	// Same number, different letter = relative major/minor (excellent)
	if k1.Number == k2.Number {
		return harmonicExcellent
	}

	// Calculate circular distance between numbers (1-12 wraps around)
	diff := abs(k1.Number - k2.Number)
	circularDist := min(diff, 12-diff)

	// ±1 number with same letter = excellent (smooth energy shift)
	if circularDist == 1 && k1.Letter == k2.Letter {
		return harmonicExcellent
	}

	// Parallel major/minor (same root note, different mode) = dramatic mood shift
	// Example: C Major (8B) ↔ C Minor (5A) - advanced technique for energy drops
	if IsParallelMajorMinor(k1, k2) {
		return harmonicDramatic
	}

	// Everything else is equally bad (not documented as valid mixing technique)
	// Whether it's 5A→6B or 5A→12A, if it's not a documented transition, it's harsh
	return harmonicIncompatible
}

// HarmonicDistance calculates the harmonic compatibility between two Camelot keys
// Returns a score where:
//
//	0 = perfect match (same key)
//	1 = excellent (±1 same letter OR relative major/minor)
//	2 = dramatic mood shift (parallel major/minor, e.g., 8B ↔ 5A)
//	10 = bad (any other transition, including invalid/missing keys)
//
// Based on official Camelot mixing documentation
func HarmonicDistance(key1, key2 string) int {
	k1, err1 := ParseCamelotKey(key1)
	k2, err2 := ParseCamelotKey(key2)

	// If either key is invalid, return large distance
	if err1 != nil || err2 != nil {
		return 999
	}

	return HarmonicDistanceParsed(k1, k2)
}

// IsCompatible returns true if two keys are harmonically compatible for mixing
// Compatible means harmonic distance <= 2
func IsCompatible(key1, key2 string) bool {
	return HarmonicDistance(key1, key2) <= 2
}

// GetCompatibleKeys returns all keys that are compatible with the given key
// Returns keys in order of compatibility (distance 0, 1, then 2)
func GetCompatibleKeys(key string) []string {
	k, err := ParseCamelotKey(key)
	if err != nil {
		return nil
	}

	var compatible []string

	// Same key (distance 0)
	compatible = append(compatible, key)

	// Relative major/minor (distance 1)
	otherLetter := byte('B')
	if k.Letter == 'B' {
		otherLetter = 'A'
	}
	compatible = append(compatible, fmt.Sprintf("%d%c", k.Number, otherLetter))

	// ±1 number with same letter (distance 1)
	prevNum := k.Number - 1
	if prevNum < 1 {
		prevNum = 12
	}
	nextNum := k.Number + 1
	if nextNum > 12 {
		nextNum = 1
	}
	compatible = append(compatible, fmt.Sprintf("%d%c", prevNum, k.Letter))
	compatible = append(compatible, fmt.Sprintf("%d%c", nextNum, k.Letter))

	// ±1 number with different letter (distance 2)
	compatible = append(compatible, fmt.Sprintf("%d%c", prevNum, otherLetter))
	compatible = append(compatible, fmt.Sprintf("%d%c", nextNum, otherLetter))

	return compatible
}

// Helper functions
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
