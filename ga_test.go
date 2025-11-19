// ABOUTME: Tests for genetic algorithm sorting behavior
// ABOUTME: Validates float comparison and individual sorting with small fitness differences

package main

import (
	"slices"
	"testing"

	"playlist-sorter/playlist"
)

func TestSortOrder(t *testing.T) {
	scores := []float64{0.5, 0.1, 0.9, 0.3}

	// Use proper float comparison, not int() conversion
	slices.SortFunc(scores, func(a, b float64) int {
		if a < b {
			return -1
		} else if a > b {
			return 1
		}

		return 0
	})

	// After sort, should be ascending (lowest first)
	if scores[0] != 0.1 {
		t.Errorf("Expected scores[0] to be lowest (0.1), got %.1f", scores[0])
	}

	if scores[3] != 0.9 {
		t.Errorf("Expected scores[3] to be highest (0.9), got %.1f", scores[3])
	}

	t.Logf("Sort order: %v (lowest first = best fitness)", scores)
}

func TestIntConversionBug(t *testing.T) {
	a := 0.0630
	b := 0.0573

	result := int(a - b)
	t.Logf("int(%.4f - %.4f) = %d", a, b, result)

	// This is the bug! int(0.0630 - 0.0573) = int(0.0057) = 0
	// When the difference is less than 1.0, int() truncates to 0!
	if result == 0 {
		t.Logf("BUG FOUND: Small differences get truncated to 0!")
	}
}

func TestIndividualSortingWithSmallDifferences(t *testing.T) {
	individuals := []Individual{
		{Score: 0.0630},
		{Score: 0.0573},
		{Score: 0.0650},
		{Score: 0.0590},
	}

	slices.SortFunc(individuals, func(a Individual, b Individual) int {
		if a.Score < b.Score {
			return -1
		} else if a.Score > b.Score {
			return 1
		}

		return 0
	})

	// After proper sort, should be ascending (lowest/best first)
	if individuals[0].Score != 0.0573 {
		t.Errorf("Expected individuals[0] to be best (0.0573), got %.4f", individuals[0].Score)
	}

	if individuals[3].Score != 0.0650 {
		t.Errorf("Expected individuals[3] to be worst (0.0650), got %.4f", individuals[3].Score)
	}

	t.Logf("Sorted individuals: %.4f, %.4f, %.4f, %.4f",
		individuals[0].Score, individuals[1].Score, individuals[2].Score, individuals[3].Score)
}

// TestOrderCrossover verifies OX crossover produces valid permutations (no duplicates)
func TestOrderCrossover(t *testing.T) {
	// Create test tracks with unique paths (crossover uses Path as key)
	tracks := make([]playlist.Track, 10)
	for i := range tracks {
		tracks[i] = playlist.Track{
			Index: i,
			Path:  string(rune('A' + i)), // A, B, C, ...
		}
	}

	parent1 := make([]playlist.Track, 10)
	parent2 := make([]playlist.Track, 10)
	child := make([]playlist.Track, 10)

	// Parent1: [0,1,2,3,4,5,6,7,8,9]
	copy(parent1, tracks)

	// Parent2: [9,8,7,6,5,4,3,2,1,0] (reversed)
	for i := range tracks {
		parent2[i] = tracks[9-i]
	}

	// Create reusable map for crossover (avoid allocations)
	present := make(map[string]bool, 10)

	// Run crossover multiple times (it's randomized)
	for trial := range 100 {
		orderCrossover(child, parent1, parent2, present)

		// Verify child is a valid permutation (no duplicates, all indices present)
		seen := make(map[int]bool)
		for _, track := range child {
			if seen[track.Index] {
				t.Fatalf("Trial %d: Duplicate index %d in child", trial, track.Index)
			}

			seen[track.Index] = true
		}

		if len(seen) != 10 {
			t.Fatalf("Trial %d: Child has %d unique tracks, want 10", trial, len(seen))
		}

		// Verify all indices 0-9 are present
		for i := range 10 {
			if !seen[i] {
				t.Fatalf("Trial %d: Missing index %d in child", trial, i)
			}
		}
	}
}

// TestReverseSegment verifies segment reversal works correctly
func TestReverseSegment(t *testing.T) {
	tests := []struct {
		name  string
		input []int
		start int
		end   int
		want  []int
	}{
		{
			name:  "reverse middle",
			input: []int{0, 1, 2, 3, 4, 5},
			start: 2,
			end:   4,
			want:  []int{0, 1, 4, 3, 2, 5},
		},
		{
			name:  "reverse beginning",
			input: []int{0, 1, 2, 3, 4},
			start: 0,
			end:   2,
			want:  []int{2, 1, 0, 3, 4},
		},
		{
			name:  "reverse end",
			input: []int{0, 1, 2, 3, 4},
			start: 3,
			end:   4,
			want:  []int{0, 1, 2, 4, 3},
		},
		{
			name:  "reverse all",
			input: []int{0, 1, 2, 3},
			start: 0,
			end:   3,
			want:  []int{3, 2, 1, 0},
		},
		{
			name:  "single element (no-op)",
			input: []int{0, 1, 2},
			start: 1,
			end:   1,
			want:  []int{0, 1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert ints to tracks
			tracks := make([]playlist.Track, len(tt.input))
			for i, idx := range tt.input {
				tracks[i] = playlist.Track{Index: idx}
			}

			// Apply reversal
			reverseSegment(tracks, tt.start, tt.end)

			// Check result
			for i, track := range tracks {
				if track.Index != tt.want[i] {
					t.Errorf("Position %d: got index %d, want %d", i, track.Index, tt.want[i])
				}
			}
		})
	}
}
