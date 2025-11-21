// ABOUTME: TUI mode configuration and command-line options
// ABOUTME: Defines input parameters for running the TUI

package tui

// Options contains configuration for running the TUI
type Options struct {
	PlaylistPath string // Path to input playlist
	OutputPath   string // Path for saving (defaults to PlaylistPath)
	DryRun       bool   // If true, don't save changes to disk
	DebugLog     bool   // Enable debug logging to file
}

// Dependencies holds all external dependencies for the TUI
// This allows for clean dependency injection and easy testing
type Dependencies struct {
	ConfigProvider ConfigProvider
	GARunner       GARunner
	PlaylistLoader PlaylistLoader
	PlaylistWriter PlaylistWriter
	Logger         Logger
	ConfigPath     string
}
