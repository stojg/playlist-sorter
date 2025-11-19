// ABOUTME: Tests for harmonic mixing functions
// ABOUTME: Validates Camelot wheel calculations including parallel major/minor detection

package playlist

import "testing"

// TestIsParallelMajorMinor tests detection of parallel major/minor key relationships
func TestIsParallelMajorMinor(t *testing.T) {
	// Test all 12 parallel major/minor pairs (same root note, different mode)
	parallelPairs := []struct {
		minor string // A (minor key)
		major string // B (major key)
	}{
		{"1A", "4B"},  // Ab minor ↔ Ab major
		{"2A", "5B"},  // Eb minor ↔ Eb major
		{"3A", "6B"},  // Bb minor ↔ Bb major
		{"4A", "7B"},  // F minor ↔ F major
		{"5A", "8B"},  // C minor ↔ C major
		{"6A", "9B"},  // G minor ↔ G major
		{"7A", "10B"}, // D minor ↔ D major
		{"8A", "11B"}, // A minor ↔ A major
		{"9A", "12B"}, // E minor ↔ E major
		{"10A", "1B"}, // B minor ↔ B major
		{"11A", "2B"}, // F# minor ↔ F# major
		{"12A", "3B"}, // Db minor ↔ Db major
	}

	for _, pair := range parallelPairs {
		k1, _ := ParseCamelotKey(pair.minor)
		k2, _ := ParseCamelotKey(pair.major)

		// Test both directions
		if !IsParallelMajorMinor(k1, k2) {
			t.Errorf("IsParallelMajorMinor(%s, %s) = false, want true", pair.minor, pair.major)
		}
		if !IsParallelMajorMinor(k2, k1) {
			t.Errorf("IsParallelMajorMinor(%s, %s) = false, want true", pair.major, pair.minor)
		}
	}
}

// TestIsParallelMajorMinor_NotParallel tests that non-parallel keys return false
func TestIsParallelMajorMinor_NotParallel(t *testing.T) {
	testCases := []struct {
		key1 string
		key2 string
		desc string
	}{
		{"8A", "8B", "relative major/minor (same number)"},
		{"8A", "9A", "adjacent keys (same letter)"},
		{"8A", "9B", "adjacent keys (different letter)"},
		{"8A", "11A", "distant keys (same letter)"},
		{"8A", "1B", "non-parallel different letter"},
		{"5A", "7B", "close but not parallel"},
	}

	for _, tc := range testCases {
		k1, _ := ParseCamelotKey(tc.key1)
		k2, _ := ParseCamelotKey(tc.key2)

		if IsParallelMajorMinor(k1, k2) {
			t.Errorf("IsParallelMajorMinor(%s, %s) = true, want false (%s)", tc.key1, tc.key2, tc.desc)
		}
	}
}

// TestIsParallelMajorMinor_NilKeys tests handling of nil keys
func TestIsParallelMajorMinor_NilKeys(t *testing.T) {
	k1, _ := ParseCamelotKey("5A")

	if IsParallelMajorMinor(nil, nil) {
		t.Error("IsParallelMajorMinor(nil, nil) = true, want false")
	}
	if IsParallelMajorMinor(k1, nil) {
		t.Error("IsParallelMajorMinor(k1, nil) = true, want false")
	}
	if IsParallelMajorMinor(nil, k1) {
		t.Error("IsParallelMajorMinor(nil, k1) = true, want false")
	}
}

// TestHarmonicDistanceParsed_ParallelMajorMinor tests distance calculation for parallel keys
func TestHarmonicDistanceParsed_ParallelMajorMinor(t *testing.T) {
	testCases := []struct {
		key1 string
		key2 string
		want int
	}{
		// Parallel major/minor pairs should return distance 2 (dramatic but valid)
		{"5A", "8B", 2},  // C minor ↔ C major
		{"8B", "5A", 2},  // C major ↔ C minor
		{"4A", "7B", 2},  // F minor ↔ F major
		{"9A", "12B", 2}, // E minor ↔ E major
		{"12A", "3B", 2}, // Db minor ↔ Db major

		// Documented good transitions
		{"8A", "8B", 1}, // Relative major/minor (same number)
		{"8A", "9A", 1}, // Adjacent same letter
		{"8A", "7A", 1}, // Adjacent same letter
		{"8A", "8A", 0}, // Same key

		// Undocumented transitions - all equally bad (distance 10)
		{"8A", "9B", 10},  // Adjacent different letter
		{"8A", "10B", 10}, // Distance 2 on wheel, different letter
		{"8A", "11A", 10}, // Distance 3 on wheel, same letter
		{"8A", "1A", 10},  // Distance 5 on wheel, same letter
		{"5A", "6B", 10},  // Adjacent different letter
	}

	for _, tc := range testCases {
		k1, _ := ParseCamelotKey(tc.key1)
		k2, _ := ParseCamelotKey(tc.key2)
		got := HarmonicDistanceParsed(k1, k2)

		if got != tc.want {
			t.Errorf("HarmonicDistanceParsed(%s, %s) = %d, want %d", tc.key1, tc.key2, got, tc.want)
		}
	}
}

// TestHarmonicDistance_ParallelMajorMinor tests string-based distance calculation
func TestHarmonicDistance_ParallelMajorMinor(t *testing.T) {
	testCases := []struct {
		key1 string
		key2 string
		want int
	}{
		{"5A", "8B", 2},  // C minor ↔ C major (parallel major/minor)
		{"8B", "5A", 2},  // C major ↔ C minor (parallel major/minor)
		{"10A", "1B", 2}, // B minor ↔ B major (parallel major/minor)
	}

	for _, tc := range testCases {
		got := HarmonicDistance(tc.key1, tc.key2)
		if got != tc.want {
			t.Errorf("HarmonicDistance(%s, %s) = %d, want %d", tc.key1, tc.key2, got, tc.want)
		}
	}
}
