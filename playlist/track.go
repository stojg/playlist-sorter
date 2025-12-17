// ABOUTME: Defines Track struct and metadata fetching directly from audio files
// ABOUTME: Provides functions to read file tags for track information including key, artist, album, energy, BPM

package playlist

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/dhowden/tag"
)

// Track represents a music track with metadata needed for sorting
type Track struct {
	Path      string      // Relative path in playlist (e.g., "Aperio/Dreams/00 Dreams.mp3")
	Key       string      // Camelot key (e.g., "8A") - for display
	ParsedKey *CamelotKey // Pre-parsed key for fast harmonic distance calculations
	Artist    string      // Artist name
	Album     string      // Album name
	Title     string      // Track title
	Genre     string      // Genre from ID3 tags (empty if not available)
	Energy    int         // Energy level 1-10 (0 if not available)
	BPM       float64     // Beats per minute (0 if not available)
	Index     int         // Index in original tracks slice (for fast cache lookups)
}

// Breakdown shows the individual fitness components for playlist optimization.
// Single source of truth - used by both GA and TUI (no duplication).
type Breakdown struct {
	Total        float64 // Sum of all weighted components
	Harmonic     float64 // Harmonic distance penalties
	EnergyDelta  float64 // Energy change penalties
	BPMDelta     float64 // BPM difference penalties
	GenreChange  float64 // Genre change/clustering (can be negative for clustering)
	SameArtist   float64 // Same artist penalties
	SameAlbum    float64 // Same album penalties
	PositionBias float64 // Low energy position bias reward
}

// Compile regexes once at package initialization
var (
	keyRegex    = regexp.MustCompile(`(\d+[AB])\s*-\s*Energy`)
	energyRegex = regexp.MustCompile(`Energy\s+(\d+)`)
)

// GetTrackMetadata fetches metadata for a track by reading the file directly.
// The trackPath can be absolute or relative. Relative paths are resolved against
// the provided baseDir (typically the playlist's directory).
func GetTrackMetadata(trackPath string, baseDir string) (*Track, error) {
	// If path is already absolute, use it as-is; otherwise resolve against base directory
	fullPath := trackPath
	if !filepath.IsAbs(trackPath) && baseDir != "" {
		fullPath = filepath.Join(baseDir, trackPath)
	}

	// Open the audio file
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Read metadata tags
	metadata, err := tag.ReadFrom(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	// Get standard tags
	artist := metadata.Artist()
	album := metadata.Album()
	title := metadata.Title()
	genre := metadata.Genre()
	comments := metadata.Comment()

	// If title is empty, use filename
	if title == "" {
		title = filepath.Base(trackPath)
	}

	// Get BPM from custom tag (varies by format)
	var bpm float64

	if raw := metadata.Raw(); raw != nil {
		// Common BPM tag names across formats
		for _, key := range []string{"BPM", "TBPM", "bpm", "tempo"} {
			if val, exists := raw[key]; exists {
				// Try different type conversions
				switch v := val.(type) {
				case string:
					bpm, _ = strconv.ParseFloat(v, 64)
				case int:
					bpm = float64(v)
				case float64:
					bpm = v
				}

				if bpm > 0 {
					break
				}
			}
		}
	}

	// Extract Camelot key and energy from comments (format: "8A - Energy 6")
	key := extractKey(comments)
	energy := extractEnergy(comments)

	// Parse key once and store it for fast lookups
	parsedKey, _ := ParseCamelotKey(key)

	return &Track{
		Path:      trackPath,
		Key:       key,
		ParsedKey: parsedKey,
		Artist:    artist,
		Album:     album,
		Title:     title,
		Genre:     genre,
		Energy:    energy,
		BPM:       bpm,
	}, nil
}

// extractKey extracts Camelot key from comments string
// Example: "8A - Energy 6" -> "8A"
func extractKey(comments string) string {
	matches := keyRegex.FindStringSubmatch(comments)
	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}

// extractEnergy extracts energy level from comments string
// Example: "8A - Energy 6" -> 6
func extractEnergy(comments string) int {
	matches := energyRegex.FindStringSubmatch(comments)
	if len(matches) > 1 {
		energy, err := strconv.Atoi(matches[1])
		if err == nil {
			return energy
		}
	}

	return 0
}

// String returns a formatted string representation of the track
func (t *Track) String() string {
	return fmt.Sprintf("%-30s - Key: %-3s Energy: %d BPM: %.0f", t.Artist, t.Key, t.Energy, t.BPM)
}
