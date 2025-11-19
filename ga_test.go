// ABOUTME: Tests for genetic algorithm sorting behavior
// ABOUTME: Validates float comparison and individual sorting with small fitness differences

package main

import (
	"slices"
	"testing"
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
