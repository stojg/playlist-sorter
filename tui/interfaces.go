// ABOUTME: Interfaces defining dependencies for the TUI package
// ABOUTME: Allows clean separation and easy testing with mocks

package tui

import (
	"context"

	"playlist-sorter/config"
	"playlist-sorter/playlist"
)

// ConfigProvider provides thread-safe access to GA configuration
type ConfigProvider interface {
	Get() config.GAConfig
	Update(cfg config.GAConfig)
}

// GARunner executes the genetic algorithm with progress updates
type GARunner interface {
	Run(ctx context.Context, tracks []playlist.Track, cfg ConfigProvider, updates chan<- Update, epoch int)
}

// PlaylistLoader loads playlists from disk with validation
type PlaylistLoader interface {
	Load(path string, requireMultiple bool) ([]playlist.Track, error)
}

// PlaylistWriter saves playlists to disk
type PlaylistWriter interface {
	Write(path string, tracks []playlist.Track) error
}

// Logger provides debug logging capability
type Logger interface {
	Debugf(format string, args ...interface{})
}

// Update represents a progress update from the GA
type Update struct {
	BestPlaylist []playlist.Track
	BestFitness  float64
	Breakdown    Breakdown
	Generation   int
	GenPerSec    float64
	Epoch        int
}

// Breakdown shows individual fitness component contributions
type Breakdown struct {
	Total        float64
	Harmonic     float64
	EnergyDelta  float64
	BPMDelta     float64
	GenreChange  float64
	SameArtist   float64
	SameAlbum    float64
	PositionBias float64
}
