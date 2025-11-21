// ABOUTME: Unit tests for TUI model behavior
// ABOUTME: Tests model initialization, state management, and core operations

package tui

import (
	"context"
	"testing"

	"playlist-sorter/config"
	"playlist-sorter/playlist"
)

// createTestModel creates a model with mock dependencies for testing
func createTestModel(tracks []playlist.Track) model {
	opts := Options{
		PlaylistPath: "test.m3u8",
		OutputPath:   "test_output.m3u8",
		DryRun:       false,
	}

	sharedCfg := &config.SharedConfig{}
	sharedCfg.Update(config.DefaultConfig())

	// Mock functions for testing
	mockRunGA := func(_ context.Context, _ []playlist.Track, _ chan<- Update, _ int) {
		// Don't send any updates in tests
	}

	mockLoadPlaylist := func(_ string, _ bool) ([]playlist.Track, error) {
		return tracks, nil
	}

	mockWritePlaylist := func(_ string, _ []playlist.Track) error {
		return nil
	}

	mockDebugf := func(_ string, _ ...interface{}) {
		// Silent in tests
	}

	return initModel(tracks, opts, sharedCfg, mockRunGA, mockLoadPlaylist, mockWritePlaylist, mockDebugf, "/tmp/test_config.toml")
}

// createTestTracks creates sample tracks for testing
func createTestTracks(count int) []playlist.Track {
	tracks := make([]playlist.Track, count)
	for i := range tracks {
		tracks[i] = playlist.Track{
			Index:  i,
			Path:   string(rune('A' + i)),
			Title:  string(rune('A' + i)),
			Artist: "Test Artist",
			Album:  "Test Album",
			Key:    "1A",
			BPM:    120.0,
			Energy: 50,
		}
	}

	return tracks
}

func TestModelInitialization(t *testing.T) {
	tracks := createTestTracks(5)
	m := createTestModel(tracks)

	if len(m.displayedTracks) != 5 {
		t.Errorf("Expected 5 tracks, got %d", len(m.displayedTracks))
	}

	if len(m.originalTracks) != 5 {
		t.Errorf("Expected 5 original tracks, got %d", len(m.originalTracks))
	}

	if len(m.params) != 8 {
		t.Errorf("Expected 8 parameters, got %d", len(m.params))
	}

	if m.selectedParam != 0 {
		t.Errorf("Expected selectedParam to be 0, got %d", m.selectedParam)
	}

	if m.focusedPanel != "playlist" {
		t.Errorf("Expected focusedPanel to be 'playlist', got '%s'", m.focusedPanel)
	}
}

func TestDeleteTrack(t *testing.T) {
	tracks := createTestTracks(5)
	m := createTestModel(tracks)

	// Set cursor to track 2
	m.cursorPos = 2

	// Delete track
	originalLen := len(m.displayedTracks)
	_ = m.deleteTrack()

	if len(m.displayedTracks) != originalLen-1 {
		t.Errorf("Expected %d tracks after delete, got %d", originalLen-1, len(m.displayedTracks))
	}

	if m.undoMgr.UndoSize() != 1 {
		t.Errorf("Expected 1 item in undo stack, got %d", m.undoMgr.UndoSize())
	}

	if !m.editMode {
		t.Error("Expected editMode to be true after delete")
	}
}

func TestDeleteLastTrack(t *testing.T) {
	tracks := createTestTracks(5)
	m := createTestModel(tracks)

	// Set cursor to last track
	m.cursorPos = 4

	_ = m.deleteTrack()

	if m.cursorPos != 3 {
		t.Errorf("Expected cursor to move to 3 after deleting last track, got %d", m.cursorPos)
	}
}

func TestUndo(t *testing.T) {
	tracks := createTestTracks(5)
	m := createTestModel(tracks)

	// Delete a track to create undo history
	m.cursorPos = 2
	originalTrack := m.displayedTracks[2]
	_ = m.deleteTrack()

	// Verify deletion
	if len(m.displayedTracks) != 4 {
		t.Fatalf("Expected 4 tracks after delete, got %d", len(m.displayedTracks))
	}

	// Undo the deletion
	_ = m.undo()

	// Verify restoration
	if len(m.displayedTracks) != 5 {
		t.Errorf("Expected 5 tracks after undo, got %d", len(m.displayedTracks))
	}

	if m.displayedTracks[2].Title != originalTrack.Title {
		t.Errorf("Expected track %s at position 2, got %s", originalTrack.Title, m.displayedTracks[2].Title)
	}

	if m.undoMgr.RedoSize() != 1 {
		t.Errorf("Expected 1 item in redo stack after undo, got %d", m.undoMgr.RedoSize())
	}
}

func TestRedo(t *testing.T) {
	tracks := createTestTracks(5)
	m := createTestModel(tracks)

	// Delete and then undo to setup redo stack
	m.cursorPos = 2
	_ = m.deleteTrack()
	_ = m.undo()

	// Verify we're back to 5 tracks
	if len(m.displayedTracks) != 5 {
		t.Fatalf("Expected 5 tracks after undo, got %d", len(m.displayedTracks))
	}

	// Redo the deletion
	_ = m.redo()

	// Verify deletion is reapplied
	if len(m.displayedTracks) != 4 {
		t.Errorf("Expected 4 tracks after redo, got %d", len(m.displayedTracks))
	}

	if m.undoMgr.UndoSize() != 1 {
		t.Errorf("Expected 1 item in undo stack after redo, got %d", m.undoMgr.UndoSize())
	}
}

func TestUndoRedoStackLimits(t *testing.T) {
	tracks := createTestTracks(60) // More than stack limit
	m := createTestModel(tracks)

	// Delete 55 tracks to exceed stack limit (max 50)
	for range 55 {
		m.cursorPos = 0
		_ = m.deleteTrack()
	}

	// Verify stack is capped at 50
	if m.undoMgr.UndoSize() > 50 {
		t.Errorf("Undo stack exceeded limit: got %d, max 50", m.undoMgr.UndoSize())
	}
}

func TestParameterAdjustment(t *testing.T) {
	tracks := createTestTracks(5)
	m := createTestModel(tracks)

	// Select harmonic weight parameter (index 0)
	m.selectedParam = 0
	originalValue := *m.params[0].Value

	// Increase parameter
	_ = m.increaseSelectedParam()
	newValue := *m.params[0].Value

	if newValue <= originalValue {
		t.Errorf("Expected parameter to increase from %.2f, got %.2f", originalValue, newValue)
	}

	// Verify epoch incremented (GA restart)
	if m.gaEpoch != 1 {
		t.Errorf("Expected gaEpoch to be 1 after parameter change, got %d", m.gaEpoch)
	}
}

func TestParameterBoundaries(t *testing.T) {
	tracks := createTestTracks(5)
	m := createTestModel(tracks)

	// Test max boundary - select first parameter and increase beyond max
	m.selectedParam = 0
	param := &m.params[0]
	*param.Value = param.Max

	_ = m.increaseSelectedParam()

	if *param.Value > param.Max {
		t.Errorf("Parameter exceeded max: %.2f > %.2f", *param.Value, param.Max)
	}

	// Test min boundary
	*param.Value = param.Min

	_ = m.decreaseSelectedParam()

	if *param.Value < param.Min {
		t.Errorf("Parameter went below min: %.2f < %.2f", *param.Value, param.Min)
	}
}

func TestResetToDefaults(t *testing.T) {
	tracks := createTestTracks(5)
	m := createTestModel(tracks)

	// Modify some parameters
	*m.params[0].Value = 0.5
	*m.params[1].Value = 0.7

	// Reset to defaults
	_ = m.resetToDefaults()

	defaults := config.DefaultConfig()
	if *m.params[0].Value != defaults.HarmonicWeight {
		t.Errorf("Parameter 0 not reset to default: got %.2f, want %.2f", *m.params[0].Value, defaults.HarmonicWeight)
	}

	if *m.params[1].Value != defaults.EnergyDeltaWeight {
		t.Errorf("Parameter 1 not reset to default: got %.2f, want %.2f", *m.params[1].Value, defaults.EnergyDeltaWeight)
	}
}
