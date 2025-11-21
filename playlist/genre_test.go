// ABOUTME: Tests for genre similarity and hierarchical matching
// ABOUTME: Verifies genre distance calculations and ancestor chain logic

package playlist

import (
	"math"
	"testing"
)

// TestGenreSimilarityIdentical verifies identical genres have 0.0 distance
func TestGenreSimilarityIdentical(t *testing.T) {
	tests := []struct {
		genre string
	}{
		{"electronic"},
		{"drum and bass"},
		{"dj drum and bass - liquid"},
		{"house"},
		{"rock"},
		{"hip hop"},
	}

	for _, tt := range tests {
		t.Run(tt.genre, func(t *testing.T) {
			similarity := GenreSimilarity(tt.genre, tt.genre)

			if similarity != genreIdentical {
				t.Errorf("Expected identical similarity (%.2f), got %.2f", genreIdentical, similarity)
			}
		})
	}
}

// TestGenreSimilarityParentChild verifies parent-child relationships
func TestGenreSimilarityParentChild(t *testing.T) {
	tests := []struct {
		name   string
		genre1 string
		genre2 string
	}{
		{"house parent", "house", "progressive house"},
		{"house parent reverse", "progressive house", "house"},
		{"dnb parent", "drum and bass", "dj drum and bass"},
		{"dnb parent reverse", "dj drum and bass", "drum and bass"},
		{"electronic parent", "electronic", "house"},
		{"electronic parent reverse", "house", "electronic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			similarity := GenreSimilarity(tt.genre1, tt.genre2)

			if similarity != genreParentChild {
				t.Errorf("Expected parent-child similarity (%.2f), got %.2f", genreParentChild, similarity)
			}
		})
	}
}

// TestGenreSimilaritySiblings verifies sibling genres (same parent)
func TestGenreSimilaritySiblings(t *testing.T) {
	tests := []struct {
		name   string
		genre1 string
		genre2 string
	}{
		{"house siblings", "progressive house", "electro house"},
		{"dnb siblings", "dj drum and bass", "jungle"},
		{"electronic siblings", "house", "techno"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			similarity := GenreSimilarity(tt.genre1, tt.genre2)

			if similarity != genreSiblings {
				t.Errorf("Expected sibling similarity (%.2f), got %.2f", genreSiblings, similarity)
			}
		})
	}
}

// TestGenreSimilaritySameRoot verifies genres with common ancestor
func TestGenreSimilaritySameRoot(t *testing.T) {
	tests := []struct {
		name   string
		genre1 string
		genre2 string
	}{
		{"electronic cousins", "progressive house", "drum and bass"},
		{"electronic cousins 2", "techno", "jungle"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			similarity := GenreSimilarity(tt.genre1, tt.genre2)

			if similarity != genreSameRoot {
				t.Errorf("Expected same-root similarity (%.2f), got %.2f", genreSameRoot, similarity)
			}
		})
	}
}

// TestGenreSimilarityUnrelated verifies completely different genres
func TestGenreSimilarityUnrelated(t *testing.T) {
	tests := []struct {
		name   string
		genre1 string
		genre2 string
	}{
		{"rock vs electronic", "rock", "electronic"},
		{"hip hop vs house", "hip hop", "house"},
		{"jazz vs metal", "jazz", "metal"},
		{"classical vs drum and bass", "classical", "drum and bass"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			similarity := GenreSimilarity(tt.genre1, tt.genre2)

			if similarity != genreUnrelated {
				t.Errorf("Expected unrelated similarity (%.2f), got %.2f", genreUnrelated, similarity)
			}
		})
	}
}

// TestGenreSimilarityEmpty verifies empty genre handling
func TestGenreSimilarityEmpty(t *testing.T) {
	tests := []struct {
		name     string
		genre1   string
		genre2   string
		expected float64
	}{
		{"both empty", "", "", genreIdentical},
		{"first empty", "", "electronic", genreUnrelated},
		{"second empty", "rock", "", genreUnrelated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			similarity := GenreSimilarity(tt.genre1, tt.genre2)

			if similarity != tt.expected {
				t.Errorf("Expected %.2f, got %.2f", tt.expected, similarity)
			}
		})
	}
}

// TestGenreSimilarityCaseInsensitive verifies case normalization
func TestGenreSimilarityCaseInsensitive(t *testing.T) {
	tests := []struct {
		name   string
		genre1 string
		genre2 string
	}{
		{"uppercase", "ELECTRONIC", "electronic"},
		{"mixed case", "ElEcTrOnIc", "electronic"},
		{"with spaces", "  electronic  ", "electronic"},
		{"all variations", "  ElEcTrOnIc  ", "electronic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			similarity := GenreSimilarity(tt.genre1, tt.genre2)

			if similarity != genreIdentical {
				t.Errorf("Expected identical (%.2f) after normalization, got %.2f", genreIdentical, similarity)
			}
		})
	}
}

// TestGetAncestorChain verifies ancestor chain construction
func TestGetAncestorChain(t *testing.T) {
	tests := []struct {
		name          string
		genre         string
		expectedChain []string
	}{
		{
			name:          "liquid dnb full chain",
			genre:         "dj drum and bass - liquid",
			expectedChain: []string{"dj drum and bass - liquid", "dj drum and bass", "drum and bass", "electronic"},
		},
		{
			name:          "house chain",
			genre:         "progressive house",
			expectedChain: []string{"progressive house", "house", "electronic"},
		},
		{
			name:          "top level genre",
			genre:         "electronic",
			expectedChain: []string{"electronic"},
		},
		{
			name:          "unknown genre",
			genre:         "unknown",
			expectedChain: []string{"unknown"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := getAncestorChain(tt.genre)

			if len(chain) != len(tt.expectedChain) {
				t.Errorf("Expected chain length %d, got %d", len(tt.expectedChain), len(chain))
				t.Logf("Expected: %v", tt.expectedChain)
				t.Logf("Got: %v", chain)
				return
			}

			for i := range chain {
				if chain[i] != tt.expectedChain[i] {
					t.Errorf("Position %d: expected %s, got %s", i, tt.expectedChain[i], chain[i])
				}
			}
		})
	}
}

// TestGenreSimilaritySymmetric verifies similarity is symmetric (A->B == B->A)
func TestGenreSimilaritySymmetric(t *testing.T) {
	tests := []struct {
		genre1 string
		genre2 string
	}{
		{"electronic", "house"},
		{"drum and bass", "jungle"},
		{"rock", "metal"},
		{"hip hop", "rap"},
	}

	for _, tt := range tests {
		t.Run(tt.genre1+"-"+tt.genre2, func(t *testing.T) {
			sim1 := GenreSimilarity(tt.genre1, tt.genre2)
			sim2 := GenreSimilarity(tt.genre2, tt.genre1)

			if sim1 != sim2 {
				t.Errorf("Similarity not symmetric: %s->%s = %.2f, %s->%s = %.2f",
					tt.genre1, tt.genre2, sim1, tt.genre2, tt.genre1, sim2)
			}
		})
	}
}

// TestGenreSimilarityRange verifies all similarities are in [0.0, 1.0] range
func TestGenreSimilarityRange(t *testing.T) {
	genres := []string{
		"electronic", "house", "progressive house",
		"drum and bass", "jungle", "dj drum and bass - liquid",
		"rock", "metal", "punk",
		"hip hop", "rap", "jazz",
		"", // Empty genre
	}

	for _, g1 := range genres {
		for _, g2 := range genres {
			similarity := GenreSimilarity(g1, g2)

			if similarity < 0.0 || similarity > 1.0 {
				t.Errorf("Similarity out of range [0.0, 1.0]: %s -> %s = %.2f", g1, g2, similarity)
			}

			// Check for NaN
			if math.IsNaN(similarity) {
				t.Errorf("Similarity is NaN: %s -> %s", g1, g2)
			}
		}
	}
}
