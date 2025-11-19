// ABOUTME: Shared initialization code for all modes (CLI, TUI, View)
// ABOUTME: Provides common playlist loading, config setup, and validation logic

package main

import (
	"fmt"
	"log"
	"os"

	"playlist-sorter/config"
	"playlist-sorter/playlist"
)

// Debug logger - writes to file for debugging
var debugLog *log.Logger

// InitDebugLog initializes debug logging to a file
func InitDebugLog(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
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

// PlaylistOptions contains options for loading playlists
type PlaylistOptions struct {
	Path    string
	Verbose bool
}

// PlaylistData contains the loaded playlist and associated data
type PlaylistData struct {
	Tracks       []playlist.Track
	Config       config.GAConfig
	SharedConfig *SharedConfig
}

// LoadPlaylistForMode loads a playlist with edge case validation and index assignment
// Returns error if playlist is empty or has only one track (unless allowSingleTrack is true)
func LoadPlaylistForMode(opts PlaylistOptions, allowSingleTrack bool) ([]playlist.Track, error) {
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
		return nil, fmt.Errorf("playlist is empty")
	}

	if len(tracks) == 1 && !allowSingleTrack {
		return nil, fmt.Errorf("playlist has only one track, nothing to optimize")
	}

	// Assign index values to tracks
	for i := range tracks {
		tracks[i].Index = i
	}

	return tracks, nil
}

// InitializePlaylist performs full initialization: load playlist, load config, build edge cache
// This is used by CLI and TUI modes that need full optimization setup
func InitializePlaylist(opts PlaylistOptions) (*PlaylistData, error) {
	// Load and validate playlist
	tracks, err := LoadPlaylistForMode(opts, false)
	if err != nil {
		return nil, err
	}

	// Load config
	cfg, _ := config.LoadConfig(config.GetConfigPath())

	// Wrap config in SharedConfig for thread-safe access
	sharedConfig := &SharedConfig{
		config: cfg,
	}

	// Build edge fitness cache (required for fitness calculations)
	buildEdgeFitnessCache(tracks)

	return &PlaylistData{
		Tracks:       tracks,
		Config:       cfg,
		SharedConfig: sharedConfig,
	}, nil
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
