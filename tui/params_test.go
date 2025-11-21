// ABOUTME: Tests for ParamManager parameter adjustment and navigation
// ABOUTME: Verifies boundary checking, integer/float handling, and reset functionality

package tui

import (
	"fmt"
	"testing"

	"playlist-sorter/config"
)

func TestParamManager_Selection(t *testing.T) {
	tests := []struct {
		name          string
		paramCount    int
		initialIndex  int
		operation     string
		expectedIndex int
	}{
		{"select next", 5, 0, "next", 1},
		{"select next at end", 5, 4, "next", 4},
		{"select previous", 5, 2, "prev", 1},
		{"select previous at start", 5, 0, "prev", 0},
		{"set valid index", 5, 0, "set:3", 3},
		{"set invalid negative", 5, 2, "set:-1", 2},
		{"set invalid too high", 5, 2, "set:10", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := createTestParams(tt.paramCount)
			pm := NewParamManager(params)
			pm.SetSelected(tt.initialIndex)

			switch tt.operation {
			case "next":
				pm.SelectNext()
			case "prev":
				pm.SelectPrevious()
			default:
				if tt.operation[:4] == "set:" {
					var idx int
					if _, err := fmt.Sscanf(tt.operation, "set:%d", &idx); err == nil {
						pm.SetSelected(idx)
					}
				}
			}

			if pm.Selected() != tt.expectedIndex {
				t.Errorf("Expected index %d, got %d", tt.expectedIndex, pm.Selected())
			}
		})
	}
}

func TestParamManager_IncreaseFloat(t *testing.T) {
	val := 0.5
	param := Parameter{
		Name:  "test",
		Value: &val,
		Min:   0.0,
		Max:   1.0,
		Step:  0.1,
		IsInt: false,
	}

	pm := NewParamManager([]Parameter{param})

	tests := []struct {
		name         string
		initialVal   float64
		expectChange bool
		expectedVal  float64
	}{
		{"increase from middle", 0.5, true, 0.6},
		{"increase to max", 0.9, true, 1.0},
		{"increase at max", 1.0, false, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			*pm.params[0].Value = tt.initialVal

			changed := pm.Increase()

			if changed != tt.expectChange {
				t.Errorf("Expected changed=%v, got %v", tt.expectChange, changed)
			}

			if *pm.params[0].Value != tt.expectedVal {
				t.Errorf("Expected value %.2f, got %.2f", tt.expectedVal, *pm.params[0].Value)
			}
		})
	}
}

func TestParamManager_DecreaseFloat(t *testing.T) {
	val := 0.5
	param := Parameter{
		Name:  "test",
		Value: &val,
		Min:   0.0,
		Max:   1.0,
		Step:  0.1,
		IsInt: false,
	}

	pm := NewParamManager([]Parameter{param})

	tests := []struct {
		name         string
		initialVal   float64
		expectChange bool
		expectedVal  float64
	}{
		{"decrease from middle", 0.5, true, 0.4},
		{"decrease to min", 0.1, true, 0.0},
		{"decrease at min", 0.0, false, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			*pm.params[0].Value = tt.initialVal

			changed := pm.Decrease()

			if changed != tt.expectChange {
				t.Errorf("Expected changed=%v, got %v", tt.expectChange, changed)
			}

			if *pm.params[0].Value != tt.expectedVal {
				t.Errorf("Expected value %.2f, got %.2f", tt.expectedVal, *pm.params[0].Value)
			}
		})
	}
}

func TestParamManager_FloatPrecisionClamping(t *testing.T) {
	val := 0.05
	param := Parameter{
		Name:  "test",
		Value: &val,
		Min:   0.0,
		Max:   1.0,
		Step:  0.05,
		IsInt: false,
	}

	pm := NewParamManager([]Parameter{param})

	// Decrease from 0.05 to 0.0 - this could result in floating point error
	// The implementation clamps values very close to min (within 0.0001)
	changed := pm.Decrease()

	if !changed {
		t.Error("Expected decrease to succeed")
	}

	// Should be clamped to exactly 0.0, not a tiny negative number
	if *pm.params[0].Value != 0.0 {
		t.Errorf("Expected value to be 0.0, got %.10f", *pm.params[0].Value)
	}
}

func TestParamManager_IncreaseInteger(t *testing.T) {
	val := 50
	param := Parameter{
		Name:     "test_int",
		IntValue: &val,
		Min:      0.0,
		Max:      100.0,
		Step:     10.0,
		IsInt:    true,
	}

	pm := NewParamManager([]Parameter{param})

	tests := []struct {
		name         string
		initialVal   int
		expectChange bool
		expectedVal  int
	}{
		{"increase from middle", 50, true, 60},
		{"increase to max", 90, true, 100},
		{"increase at max", 100, false, 100},
		{"increase would exceed max", 95, false, 95},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			*pm.params[0].IntValue = tt.initialVal

			changed := pm.Increase()

			if changed != tt.expectChange {
				t.Errorf("Expected changed=%v, got %v", tt.expectChange, changed)
			}

			if *pm.params[0].IntValue != tt.expectedVal {
				t.Errorf("Expected value %d, got %d", tt.expectedVal, *pm.params[0].IntValue)
			}
		})
	}
}

func TestParamManager_DecreaseInteger(t *testing.T) {
	val := 50
	param := Parameter{
		Name:     "test_int",
		IntValue: &val,
		Min:      0.0,
		Max:      100.0,
		Step:     10.0,
		IsInt:    true,
	}

	pm := NewParamManager([]Parameter{param})

	tests := []struct {
		name         string
		initialVal   int
		expectChange bool
		expectedVal  int
	}{
		{"decrease from middle", 50, true, 40},
		{"decrease to min", 10, true, 0},
		{"decrease at min", 0, false, 0},
		{"decrease would go below min", 5, false, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			*pm.params[0].IntValue = tt.initialVal

			changed := pm.Decrease()

			if changed != tt.expectChange {
				t.Errorf("Expected changed=%v, got %v", tt.expectChange, changed)
			}

			if *pm.params[0].IntValue != tt.expectedVal {
				t.Errorf("Expected value %d, got %d", tt.expectedVal, *pm.params[0].IntValue)
			}
		})
	}
}

func TestParamManager_ResetToDefaults(t *testing.T) {
	// Create parameters matching the expected GA config structure
	cfg := config.GAConfig{
		HarmonicWeight:       0.5,
		EnergyDeltaWeight:    0.3,
		BPMDeltaWeight:       0.2,
		GenreWeight:          0.1,
		SameArtistPenalty:    0.4,
		SameAlbumPenalty:     0.6,
		LowEnergyBiasPortion: 0.15,
		LowEnergyBiasWeight:  0.25,
	}

	params := []Parameter{
		{Name: "Harmonic Weight", Value: &cfg.HarmonicWeight, Min: 0, Max: 1, Step: 0.05},
		{Name: "Energy Delta Weight", Value: &cfg.EnergyDeltaWeight, Min: 0, Max: 1, Step: 0.05},
		{Name: "BPM Delta Weight", Value: &cfg.BPMDeltaWeight, Min: 0, Max: 1, Step: 0.05},
		{Name: "Genre Weight", Value: &cfg.GenreWeight, Min: -1, Max: 1, Step: 0.1},
		{Name: "Same Artist Penalty", Value: &cfg.SameArtistPenalty, Min: 0, Max: 1, Step: 0.05},
		{Name: "Same Album Penalty", Value: &cfg.SameAlbumPenalty, Min: 0, Max: 1, Step: 0.05},
		{Name: "Low Energy Bias Portion", Value: &cfg.LowEnergyBiasPortion, Min: 0, Max: 1, Step: 0.05},
		{Name: "Low Energy Bias Weight", Value: &cfg.LowEnergyBiasWeight, Min: 0, Max: 1, Step: 0.05},
	}

	pm := NewParamManager(params)

	// Modify all parameters
	*pm.params[0].Value = 0.9
	*pm.params[1].Value = 0.8
	*pm.params[2].Value = 0.7
	*pm.params[3].Value = 0.6
	*pm.params[4].Value = 0.5
	*pm.params[5].Value = 0.4
	*pm.params[6].Value = 0.3
	*pm.params[7].Value = 0.2

	// Reset to defaults
	defaults := config.DefaultConfig()
	pm.ResetToDefaults(defaults)

	// Verify all parameters restored
	tests := []struct {
		index    int
		expected float64
		name     string
	}{
		{0, defaults.HarmonicWeight, "HarmonicWeight"},
		{1, defaults.EnergyDeltaWeight, "EnergyDeltaWeight"},
		{2, defaults.BPMDeltaWeight, "BPMDeltaWeight"},
		{3, defaults.GenreWeight, "GenreWeight"},
		{4, defaults.SameArtistPenalty, "SameArtistPenalty"},
		{5, defaults.SameAlbumPenalty, "SameAlbumPenalty"},
		{6, defaults.LowEnergyBiasPortion, "LowEnergyBiasPortion"},
		{7, defaults.LowEnergyBiasWeight, "LowEnergyBiasWeight"},
	}

	for _, tt := range tests {
		if *pm.params[tt.index].Value != tt.expected {
			t.Errorf("%s not reset: got %.2f, want %.2f",
				tt.name, *pm.params[tt.index].Value, tt.expected)
		}
	}
}

func TestParamManager_GetMethods(t *testing.T) {
	params := createTestParams(5)
	pm := NewParamManager(params)

	// Test Len
	if pm.Len() != 5 {
		t.Errorf("Expected length 5, got %d", pm.Len())
	}

	// Test Get with valid index
	param := pm.Get(2)
	if param == nil {
		t.Fatal("Expected non-nil parameter")
	}
	if param.Name != params[2].Name {
		t.Errorf("Expected parameter %s, got %s", params[2].Name, param.Name)
	}

	// Test Get with invalid indices
	if pm.Get(-1) != nil {
		t.Error("Expected nil for negative index")
	}
	if pm.Get(10) != nil {
		t.Error("Expected nil for out-of-bounds index")
	}

	// Test GetSelected
	pm.SetSelected(3)
	selected := pm.GetSelected()
	if selected == nil {
		t.Fatal("Expected non-nil selected parameter")
	}
	if selected.Name != params[3].Name {
		t.Errorf("Expected selected parameter %s, got %s", params[3].Name, selected.Name)
	}

	// Test All
	all := pm.All()
	if len(all) != 5 {
		t.Errorf("Expected All() to return 5 parameters, got %d", len(all))
	}
}

func TestParamManager_BoundaryConditionsEdgeCases(t *testing.T) {
	// Test with zero step
	val := 0.5
	param := Parameter{
		Name:  "zero_step",
		Value: &val,
		Min:   0.0,
		Max:   1.0,
		Step:  0.0,
		IsInt: false,
	}

	pm := NewParamManager([]Parameter{param})

	// Should not change with zero step
	changed := pm.Increase()
	if !changed {
		t.Error("Increase with zero step should still return true (even though value doesn't change)")
	}
	if *pm.params[0].Value != 0.5 {
		t.Errorf("Value should remain 0.5, got %.2f", *pm.params[0].Value)
	}
}

// Helper function to create test parameters
func createTestParams(count int) []Parameter {
	params := make([]Parameter, count)
	for i := range params {
		val := float64(i) * 0.1
		params[i] = Parameter{
			Name:  fmt.Sprintf("param_%d", i),
			Value: &val,
			Min:   0.0,
			Max:   1.0,
			Step:  0.1,
			IsInt: false,
		}
	}
	return params
}
