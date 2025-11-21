// ABOUTME: Parameter manager for GA configuration tuning
// ABOUTME: Handles parameter value adjustments with boundary checking

package tui

import "playlist-sorter/config"

// ParamManager manages GA parameter adjustments
type ParamManager struct {
	params        []Parameter
	selectedIndex int
}

// NewParamManager creates a new parameter manager
func NewParamManager(params []Parameter) *ParamManager {
	return &ParamManager{
		params:        params,
		selectedIndex: 0,
	}
}

// Selected returns the index of the currently selected parameter
func (pm *ParamManager) Selected() int {
	return pm.selectedIndex
}

// SetSelected sets the selected parameter index
func (pm *ParamManager) SetSelected(index int) {
	if index >= 0 && index < len(pm.params) {
		pm.selectedIndex = index
	}
}

// SelectNext moves selection to the next parameter
func (pm *ParamManager) SelectNext() {
	if pm.selectedIndex < len(pm.params)-1 {
		pm.selectedIndex++
	}
}

// SelectPrevious moves selection to the previous parameter
func (pm *ParamManager) SelectPrevious() {
	if pm.selectedIndex > 0 {
		pm.selectedIndex--
	}
}

// Increase increases the selected parameter value
// Returns true if the value was changed
func (pm *ParamManager) Increase() bool {
	if pm.selectedIndex >= len(pm.params) {
		return false
	}

	param := &pm.params[pm.selectedIndex]
	if param.IsInt {
		newVal := *param.IntValue + int(param.Step)
		if float64(newVal) <= param.Max {
			*param.IntValue = newVal
			return true
		}
	} else {
		newVal := *param.Value + param.Step
		if newVal <= param.Max {
			*param.Value = newVal
			return true
		}
	}

	return false
}

// Decrease decreases the selected parameter value
// Returns true if the value was changed
func (pm *ParamManager) Decrease() bool {
	if pm.selectedIndex >= len(pm.params) {
		return false
	}

	param := &pm.params[pm.selectedIndex]
	if param.IsInt {
		newVal := *param.IntValue - int(param.Step)
		if float64(newVal) >= param.Min {
			*param.IntValue = newVal
			return true
		}
	} else {
		newVal := *param.Value - param.Step
		// Clamp to min if we're very close (handles floating point precision)
		if newVal < param.Min && newVal >= param.Min-0.0001 {
			newVal = param.Min
		}

		if newVal >= param.Min {
			*param.Value = newVal
			return true
		}
	}

	return false
}

// ResetToDefaults resets all parameters to their default values
func (pm *ParamManager) ResetToDefaults(defaults config.GAConfig) {
	// Note: This assumes parameter order matches the config fields
	// This is enforced by the initialization in model.go
	if len(pm.params) >= 8 {
		*pm.params[0].Value = defaults.HarmonicWeight
		*pm.params[1].Value = defaults.EnergyDeltaWeight
		*pm.params[2].Value = defaults.BPMDeltaWeight
		*pm.params[3].Value = defaults.GenreWeight
		*pm.params[4].Value = defaults.SameArtistPenalty
		*pm.params[5].Value = defaults.SameAlbumPenalty
		*pm.params[6].Value = defaults.LowEnergyBiasPortion
		*pm.params[7].Value = defaults.LowEnergyBiasWeight
	}
}

// Get returns the parameter at the given index
func (pm *ParamManager) Get(index int) *Parameter {
	if index >= 0 && index < len(pm.params) {
		return &pm.params[index]
	}
	return nil
}

// GetSelected returns the currently selected parameter
func (pm *ParamManager) GetSelected() *Parameter {
	return pm.Get(pm.selectedIndex)
}

// Len returns the number of parameters
func (pm *ParamManager) Len() int {
	return len(pm.params)
}

// All returns all parameters (for rendering)
func (pm *ParamManager) All() []Parameter {
	return pm.params
}
