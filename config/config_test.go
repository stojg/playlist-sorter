// ABOUTME: Tests for configuration load/save functionality
// ABOUTME: Validates TOML parsing and default config fallback behavior

package config

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.HarmonicWeight != 0.3 {
		t.Errorf("Expected HarmonicWeight 0.3, got %.2f", cfg.HarmonicWeight)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	// Create temp file
	tmpfile, err := os.CreateTemp(t.TempDir(), "playlist-sorter-*.toml")
	if err != nil {
		t.Fatal(err)
	}

	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	// Save default config
	cfg := DefaultConfig()
	if err := SaveConfig(tmpfile.Name(), cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Load it back
	loaded, err := LoadConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify values match
	if loaded.HarmonicWeight != cfg.HarmonicWeight {
		t.Errorf("HarmonicWeight mismatch: got %.2f, want %.2f", loaded.HarmonicWeight, cfg.HarmonicWeight)
	}
}

func TestLoadNonExistentConfig(t *testing.T) {
	// Loading non-existent file should return defaults without error
	cfg, err := LoadConfig("/nonexistent/path/config.toml")
	if err != nil {
		t.Errorf("Expected no error for non-existent file, got: %v", err)
	}

	// Should be default values
	defaults := DefaultConfig()
	if cfg.HarmonicWeight != defaults.HarmonicWeight {
		t.Errorf("Expected default HarmonicWeight %.2f, got %.2f", defaults.HarmonicWeight, cfg.HarmonicWeight)
	}
}
