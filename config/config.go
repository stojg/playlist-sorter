// ABOUTME: Configuration management for genetic algorithm parameters
// ABOUTME: Handles loading/saving TOML config files with fallback to defaults

package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// GAConfig holds all tunable genetic algorithm parameters
type GAConfig struct {
	// Fitness penalty weights
	HarmonicWeight    float64 `toml:"harmonic_weight"`
	SameArtistPenalty float64 `toml:"same_artist_penalty"`
	SameAlbumPenalty  float64 `toml:"same_album_penalty"`
	EnergyDeltaWeight float64 `toml:"energy_delta_weight"`
	BPMDeltaWeight    float64 `toml:"bpm_delta_weight"`
	GenreWeight       float64 `toml:"genre_weight"` // -1.0 (spread) to +1.0 (cluster)

	// Position bias
	LowEnergyBiasPortion float64 `toml:"low_energy_bias_portion"`
	LowEnergyBiasWeight  float64 `toml:"low_energy_bias_weight"`
}

// GetConfigPath returns the default config file path
// First tries current directory, then falls back to ~/.config/playlist-sorter/config.toml
func GetConfigPath() string {
	// First try current directory
	if _, err := os.Stat("./playlist-sorter.toml"); err == nil {
		return "./playlist-sorter.toml"
	}

	// Then try ~/.config/playlist-sorter/config.toml
	home, err := os.UserHomeDir()
	if err != nil {
		return "./playlist-sorter.toml"
	}

	return filepath.Join(home, ".config", "playlist-sorter", "config.toml")
}

// LoadConfig loads configuration from a TOML file
// If the file doesn't exist or fails to load, returns default config
func LoadConfig(path string) (GAConfig, error) {
	// Try to read the file
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, return defaults
			return DefaultConfig(), nil
		}
		return DefaultConfig(), fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse TOML
	var config GAConfig
	if err := toml.Unmarshal(data, &config); err != nil {
		return DefaultConfig(), fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

// SaveConfig saves configuration to a TOML file
func SaveConfig(path string, config GAConfig) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Round all float values to 2 decimal places to match UI precision
	// This prevents floating point rounding errors from accumulating
	config = roundConfigPrecision(config)

	// Create file
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("Warning: failed to close config file: %v\n", err)
		}
	}()

	// Encode config as TOML
	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// DefaultConfig returns the default GA configuration with normalized fitness weights
func DefaultConfig() GAConfig {
	return GAConfig{
		HarmonicWeight:       0.3,
		SameArtistPenalty:    0.2,
		SameAlbumPenalty:     0.2,
		EnergyDeltaWeight:    0.3,
		BPMDeltaWeight:       0.1,
		GenreWeight:          0.0,
		LowEnergyBiasPortion: 0.2,
		LowEnergyBiasWeight:  0.0,
	}
}

// roundConfigPrecision rounds all float64 fields to 2 decimal places
func roundConfigPrecision(config GAConfig) GAConfig {
	round := func(x float64) float64 {
		return float64(int(x*100+0.5)) / 100
	}

	config.HarmonicWeight = round(config.HarmonicWeight)
	config.SameArtistPenalty = round(config.SameArtistPenalty)
	config.SameAlbumPenalty = round(config.SameAlbumPenalty)
	config.EnergyDeltaWeight = round(config.EnergyDeltaWeight)
	config.BPMDeltaWeight = round(config.BPMDeltaWeight)
	config.GenreWeight = round(config.GenreWeight)
	config.LowEnergyBiasPortion = round(config.LowEnergyBiasPortion)
	config.LowEnergyBiasWeight = round(config.LowEnergyBiasWeight)

	return config
}
