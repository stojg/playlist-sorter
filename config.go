// ABOUTME: Configuration management for genetic algorithm parameters
// ABOUTME: Handles loading/saving TOML config files with fallback to defaults

package main

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

	// Position bias
	LowEnergyBiasPortion float64 `toml:"low_energy_bias_portion"`
	LowEnergyBiasWeight  float64 `toml:"low_energy_bias_weight"`

	// Mutation parameters
	MaxMutationRate  float64 `toml:"max_mutation_rate"`
	MinMutationRate  float64 `toml:"min_mutation_rate"`
	MutationDecayGen float64 `toml:"mutation_decay_gen"`
	MinSwapMutations int     `toml:"min_swap_mutations"`
	MaxSwapMutations int     `toml:"max_swap_mutations"`

	// Population parameters
	PopulationSize  int     `toml:"population_size"`
	ImmigrationRate float64 `toml:"immigration_rate"`
	ElitePercentage float64 `toml:"elite_percentage"`
	TournamentSize  int     `toml:"tournament_size"`
}

// DefaultConfig returns the default GA configuration matching the original constants
// These values are research-backed defaults optimized for playlist ordering
func DefaultConfig() GAConfig {
	return GAConfig{
		HarmonicWeight:       1.0,
		SameArtistPenalty:    5.0,
		SameAlbumPenalty:     2.0,
		EnergyDeltaWeight:    3.0,
		BPMDeltaWeight:       0.25,
		LowEnergyBiasPortion: 0.2,
		LowEnergyBiasWeight:  10.0,
		MaxMutationRate:      0.3,
		MinMutationRate:      0.1,
		MutationDecayGen:     100.0,
		MinSwapMutations:     2,
		MaxSwapMutations:     5,
		PopulationSize:       100,
		ImmigrationRate:      0.05,
		ElitePercentage:      0.03, // Reduced from 0.1 to reduce 2-opt memory copying overhead
		TournamentSize:       3,
	}
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
