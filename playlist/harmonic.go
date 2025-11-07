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
	Letter string // "A" (minor) or "B" (major)
	Number int    // 1-12
}

// Compile regex once at package initialization
var camelotKeyRegex = regexp.MustCompile(`^(\d+)([AB])$`)

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
		Letter: matches[2],
		Number: number,
	}, nil
}

// String returns the string representation of a CamelotKey
func (k *CamelotKey) String() string {
	return fmt.Sprintf("%d%s", k.Number, k.Letter)
}

// HarmonicDistanceParsed calculates harmonic compatibility using pre-parsed keys
// This is much faster than HarmonicDistance as it skips parsing
// Returns 999 if either key is nil
func HarmonicDistanceParsed(k1, k2 *CamelotKey) int {
	// If either key is invalid, return large distance
	if k1 == nil || k2 == nil {
		return 999
	}

	// Same key = perfect match
	if k1.Number == k2.Number && k1.Letter == k2.Letter {
		return 0
	}

	// Same number, different letter = relative major/minor (excellent)
	if k1.Number == k2.Number {
		return 1
	}

	// Calculate circular distance between numbers (1-12 wraps around)
	diff := abs(k1.Number - k2.Number)
	circularDist := min(diff, 12-diff)

	// ±1 number with same letter = excellent
	if circularDist == 1 && k1.Letter == k2.Letter {
		return 1
	}

	// ±1 number with different letter = acceptable but not ideal
	if circularDist == 1 {
		return 3
	}

	// Everything else scales with distance
	return circularDist + 1
}

// HarmonicDistance calculates the harmonic compatibility between two Camelot keys
// Returns a score where:
//
//	0 = perfect match (same key)
//	1 = excellent (±1 number OR relative major/minor)
//	3 = acceptable (±1 number with different letter)
//	higher = less compatible
//
// Returns 999 for invalid keys
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
	otherLetter := "B"
	if k.Letter == "B" {
		otherLetter = "A"
	}
	compatible = append(compatible, fmt.Sprintf("%d%s", k.Number, otherLetter))

	// ±1 number with same letter (distance 1)
	prevNum := k.Number - 1
	if prevNum < 1 {
		prevNum = 12
	}
	nextNum := k.Number + 1
	if nextNum > 12 {
		nextNum = 1
	}
	compatible = append(compatible, fmt.Sprintf("%d%s", prevNum, k.Letter))
	compatible = append(compatible, fmt.Sprintf("%d%s", nextNum, k.Letter))

	// ±1 number with different letter (distance 2)
	compatible = append(compatible, fmt.Sprintf("%d%s", prevNum, otherLetter))
	compatible = append(compatible, fmt.Sprintf("%d%s", nextNum, otherLetter))

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
