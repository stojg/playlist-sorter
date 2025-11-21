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

var debugLog *log.Logger

// RunOptions contains command-line options for all modes
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

// InitializePlaylist loads playlist, config, and builds edge cache for optimization
func InitializePlaylist(opts PlaylistOptions) (*OptimizationContext, error) {
	tracks, err := LoadPlaylistForMode(opts, false)
	if err != nil {
		return nil, err
	}

	cfg, _ := config.LoadConfig(config.GetConfigPath())

	sharedConfig := &config.SharedConfig{}
	sharedConfig.Update(cfg)

	gaCtx := buildEdgeFitnessCache(tracks)

	return &OptimizationContext{
		Tracks:       tracks,
		Config:       cfg,
		SharedConfig: sharedConfig,
		GACtx:        gaCtx,
	}, nil
}

// LoadPlaylistForMode loads playlist with validation and index assignment
func LoadPlaylistForMode(opts PlaylistOptions, allowSingle bool) ([]playlist.Track, error) {
	if opts.Verbose {
		fmt.Printf("Reading playlist: %s\n", opts.Path)
	}

	tracks, err := playlist.LoadPlaylistWithMetadata(opts.Path, opts.Verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to load playlist: %w", err)
	}

	if len(tracks) == 0 {
		return nil, errors.New("playlist is empty")
	}

	if len(tracks) == 1 && !allowSingle {
		return nil, errors.New("playlist has only one track, nothing to optimize")
	}

	for i := range tracks {
		tracks[i].Index = i
	}

	return tracks, nil
}

// SetupDebugLog initializes debug logging
func SetupDebugLog(filename string) error {
	if err := InitDebugLog(filename); err != nil {
		return fmt.Errorf("failed to initialize debug log: %w", err)
	}

	if filename == "playlist-sorter-debug.log" {
		fileInfo, _ := os.Stdout.Stat()
		if (fileInfo.Mode() & os.ModeCharDevice) != 0 {
			fmt.Printf("Debug logging enabled: %s\n", filename)
		}
	}

	return nil
}

// InitDebugLog initializes debug logging
func InitDebugLog(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create debug log file: %w", err)
	}

	debugLog = log.New(f, "", log.Ltime|log.Lmicroseconds)

	return nil
}

// debugf logs debug messages if enabled
func debugf(format string, args ...interface{}) {
	if debugLog != nil {
		debugLog.Printf(format, args...)
	}
}

// truncate shortens string to maxLen, adding "..." if needed
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	if maxLen <= 3 {
		return s[:maxLen]
	}

	return s[:maxLen-3] + "..."
}

// hasFitnessImproved returns true if newFitness significantly better (uses epsilon for float comparison)
func hasFitnessImproved(newFitness, oldFitness, epsilon float64) bool {
	return newFitness < oldFitness-epsilon
}
