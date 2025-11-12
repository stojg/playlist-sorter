// ABOUTME: Genre similarity calculation with hierarchical matching
// ABOUTME: Maps genres to parents and calculates distance for fuzzy genre matching

package playlist

import (
	"strings"
)

// Genre hierarchy: maps genre -> parent genre
// Built from actual beets library genres, reflecting user's organization system
var genreHierarchy = map[string]string{
	// DJ Drum and Bass family (user's DJ organization system)
	"dj drum and bass - liquid": "dj drum and bass",
	"dj drum and bass":          "drum and bass",
	"drum and bass":             "electronic",
	"jungle":                    "drum and bass", // Related to DnB

	// DJ House family
	"dj electro house": "dj house",
	"dj house":         "house",
	"electro house":    "house",
	"progressive house": "house",
	"house":            "electronic",

	// DJ Dubstep
	"dj dubstep": "electronic",

	// DJ Electro Swing
	"dj electro swing": "electro swing",
	"electro swing":    "electronic",

	// DJ Other
	"dj edm":   "electronic",
	"dj dance": "dance",
	"dj pop":   "pop",
	"dj beat":  "electronic",

	// Electronic sub-genres
	"breakbeat":   "electronic",
	"breaks":      "electronic",
	"downtempo":   "electronic",
	"electro":     "electronic",
	"electronica": "electronic",
	"synth":       "electronic",
	"synthpop":    "electronic",
	"synthwave":   "electronic",
	"techno":      "electronic",
	"trance":      "electronic",
	"trap":        "electronic",
	"garage":      "electronic",
	"rave":        "electronic",

	// Rock family
	"alternative":  "rock",
	"hard rock":    "rock",
	"heavy metal":  "rock",
	"metal":        "rock",
	"thrash metal": "metal",
	"punk":         "rock",
	"punkrock":     "rock",
	"indie":        "rock",
	"industrial":   "rock",

	// Hip Hop family
	"hiphop":           "hip hop",
	"hip-hop":          "hip hop",
	"hip-hop-rap":      "hip hop",
	"rap":              "hip hop",
	"alternative rap":  "hip hop",
	"old school rap":   "hip hop",
	"svensk hiphop":    "hip hop",

	// Jazz family
	"acid jazz funk": "jazz",
	"fusion":         "jazz",

	// Funk/Soul family
	"funk":      "funk / soul",
	"funk / soul": "",

	// Reggae family
	"reggea":       "reggae",
	"roots reggae": "reggae",
	"dub":          "reggae",

	// Top-level genres (no parent)
	"electronic": "",
	"rock":       "",
	"hip hop":    "",
	"jazz":       "",
	"classical":  "",
	"pop":        "",
	"dance":      "",
	"blues":      "",
	"country":    "",
	"reggae":     "",
	"soul":       "",
	"r&b":        "",
	"lounge":     "",
	"soundtrack": "",
	"comedy":     "",
	"world":      "",
}

// GenreSimilarity calculates similarity between two genres
// Returns 0.0 for identical genres, 1.0 for completely different
// Uses hierarchical matching: sub-genres are closer than unrelated genres
func GenreSimilarity(genre1, genre2 string) float64 {
	// Normalize genres (lowercase, trim)
	g1 := strings.ToLower(strings.TrimSpace(genre1))
	g2 := strings.ToLower(strings.TrimSpace(genre2))

	// Handle empty genres (treat as completely different)
	if g1 == "" || g2 == "" {
		if g1 == g2 {
			return 0.0 // Both empty = same
		}
		return 1.0 // One empty = different
	}

	// Exact match
	if g1 == g2 {
		return 0.0
	}

	// Get ancestor chains
	chain1 := getAncestorChain(g1)
	chain2 := getAncestorChain(g2)

	// Check if one is parent of the other
	if contains(chain1, g2) || contains(chain2, g1) {
		return 0.15 // Very close (parent/child relationship)
	}

	// Check if they share a parent
	if sharesParent(chain1, chain2) {
		return 0.3 // Close (siblings in hierarchy)
	}

	// Check if they share a grandparent (same root category)
	if sharesGrandparent(chain1, chain2) {
		return 0.7 // Somewhat related (same root)
	}

	// Completely unrelated
	return 1.0
}

// getAncestorChain returns the full ancestry chain for a genre
// Example: "liquid dnb" -> ["liquid dnb", "drum & bass", "electronic"]
func getAncestorChain(genre string) []string {
	chain := []string{genre}
	current := genre

	for {
		parent, exists := genreHierarchy[current]
		if !exists || parent == "" {
			break
		}
		chain = append(chain, parent)
		current = parent
	}

	return chain
}

// contains checks if a string is in a slice
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// sharesParent checks if two ancestor chains share an immediate parent
func sharesParent(chain1, chain2 []string) bool {
	// Check if they have a common parent (at depth 1)
	if len(chain1) > 1 && len(chain2) > 1 {
		return chain1[1] == chain2[1]
	}
	return false
}

// sharesGrandparent checks if two ancestor chains share a root category
func sharesGrandparent(chain1, chain2 []string) bool {
	// Find the deepest common ancestor
	for _, ancestor1 := range chain1[1:] { // Skip first (the genre itself)
		for _, ancestor2 := range chain2[1:] {
			if ancestor1 == ancestor2 {
				return true
			}
		}
	}
	return false
}
