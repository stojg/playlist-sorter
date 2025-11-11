package main

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.HarmonicWeight != 1.0 {
		t.Errorf("Expected HarmonicWeight 1.0, got %.2f", cfg.HarmonicWeight)
	}

	if cfg.PopulationSize != 100 {
		t.Errorf("Expected PopulationSize 100, got %d", cfg.PopulationSize)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	// Create temp file
	tmpfile, err := os.CreateTemp("", "playlist-sorter-*.toml")
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
	if loaded.PopulationSize != cfg.PopulationSize {
		t.Errorf("PopulationSize mismatch: got %d, want %d", loaded.PopulationSize, cfg.PopulationSize)
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
