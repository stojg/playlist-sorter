// ABOUTME: Shared initialization code for all modes (CLI, TUI, View)
// ABOUTME: Provides common playlist loading, config setup, and validation logic

package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"playlist-sorter/config"
	"playlist-sorter/playlist"
)

// Debug logger - writes to file for debugging
var debugLog *log.Logger

// RunOptions contains command-line options for all modes (CLI, TUI, View)
type RunOptions struct {
	PlaylistPath string
	DryRun       bool
	OutputPath   string
	DebugLog     bool
}

// PlaylistOptions contains options for loading playlists
type PlaylistOptions struct {
	Path    string
	Verbose bool
}

// OptimizationContext contains the loaded playlist and associated data
type OptimizationContext struct {
	Tracks       []playlist.Track
	Config       config.GAConfig
	SharedConfig *config.SharedConfig
	GACtx        *GAContext
}

// InitializePlaylist performs full initialization: load playlist, load config, build edge cache
// This is used by CLI and TUI modes that need full optimization setup
func InitializePlaylist(opts PlaylistOptions) (*OptimizationContext, error) {
	// Load and validate playlist (require multiple tracks for optimization)
	tracks, err := LoadPlaylistForMode(opts, false)
	if err != nil {
		return nil, err
	}

	// Load config
	cfg, _ := config.LoadConfig(config.GetConfigPath())

	// Wrap config in SharedConfig for thread-safe access
	sharedConfig := &config.SharedConfig{}
	sharedConfig.Update(cfg)

	// Build edge fitness cache (required for fitness calculations)
	gaCtx := buildEdgeFitnessCache(tracks)

	return &OptimizationContext{
		Tracks:       tracks,
		Config:       cfg,
		SharedConfig: sharedConfig,
		GACtx:        gaCtx,
	}, nil
}

// LoadPlaylistForMode loads a playlist with edge case validation and index assignment.
// If allowSingle is false, returns error for single-track playlists (optimization requires multiple tracks).
func LoadPlaylistForMode(opts PlaylistOptions, allowSingle bool) ([]playlist.Track, error) {
	// Load playlist
	if opts.Verbose {
		fmt.Printf("Reading playlist: %s\n", opts.Path)
	}

	tracks, err := playlist.LoadPlaylistWithMetadata(opts.Path, opts.Verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to load playlist: %w", err)
	}

	// Validate playlist size
	if len(tracks) == 0 {
		return nil, errors.New("playlist is empty")
	}

	if len(tracks) == 1 && !allowSingle {
		return nil, errors.New("playlist has only one track, nothing to optimize")
	}

	// Assign index values to tracks
	for i := range tracks {
		tracks[i].Index = i
	}

	return tracks, nil
}

// SetupDebugLog initializes debug logging to the specified file
func SetupDebugLog(filename string) error {
	if err := InitDebugLog(filename); err != nil {
		return fmt.Errorf("failed to initialize debug log: %w", err)
	}

	// Only print to stdout in CLI mode (verbose)
	if filename == "playlist-sorter-debug.log" {
		// Check if we're in CLI mode by seeing if stdout is terminal
		fileInfo, _ := os.Stdout.Stat()
		if (fileInfo.Mode() & os.ModeCharDevice) != 0 {
			fmt.Printf("Debug logging enabled: %s\n", filename)
		}
	}

	return nil
}

// InitDebugLog initializes debug logging to a file
func InitDebugLog(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create debug log file: %w", err)
	}

	debugLog = log.New(f, "", log.Ltime|log.Lmicroseconds)

	return nil
}

// debugf logs debug messages to file if debug logger is enabled
func debugf(format string, args ...interface{}) {
	if debugLog != nil {
		debugLog.Printf(format, args...)
	}
}

// truncate truncates a string to maxLen characters, adding "..." if needed
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	if maxLen <= 3 {
		return s[:maxLen]
	}

	return s[:maxLen-3] + "..."
}

// hasFitnessImproved returns true if newFitness is significantly better than oldFitness
// Uses epsilon threshold to avoid false positives from floating-point precision issues
func hasFitnessImproved(newFitness, oldFitness, epsilon float64) bool {
	return newFitness < oldFitness-epsilon
}
