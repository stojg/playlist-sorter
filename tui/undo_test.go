// ABOUTME: Tests for UndoManager stack operations
// ABOUTME: Verifies undo/redo behavior and stack size limits

package tui

import (
	"testing"

	"playlist-sorter/playlist"
)

func createTestState(trackCount, cursorPos int) PlaylistState {
	tracks := make([]playlist.Track, trackCount)
	for i := range tracks {
		tracks[i] = playlist.Track{
			Index: i,
			Title: string(rune('A' + i)),
		}
	}

	return PlaylistState{
		Tracks:    tracks,
		CursorPos: cursorPos,
	}
}

func TestUndoManager_PushAndUndo(t *testing.T) {
	um := NewUndoManager(50)

	// Initial state
	state1 := createTestState(5, 0)
	um.Push(state1)

	// Modified state
	state2 := createTestState(4, 1) // Deleted one track, cursor moved

	// Undo
	restored, ok := um.Undo(state2)
	if !ok {
		t.Fatal("Undo should succeed")
	}

	if len(restored.Tracks) != 5 {
		t.Errorf("Undo restored %d tracks, want 5", len(restored.Tracks))
	}

	if restored.CursorPos != 0 {
		t.Errorf("Undo restored cursor to %d, want 0", restored.CursorPos)
	}
}

func TestUndoManager_UndoEmpty(t *testing.T) {
	um := NewUndoManager(50)

	currentState := createTestState(5, 0)
	_, ok := um.Undo(currentState)

	if ok {
		t.Error("Undo should fail on empty stack")
	}
}

func TestUndoManager_Redo(t *testing.T) {
	um := NewUndoManager(50)

	// Push initial state
	state1 := createTestState(5, 0)
	um.Push(state1)

	// Undo to populate redo stack
	state2 := createTestState(4, 1)

	restored, ok := um.Undo(state2)
	if !ok {
		t.Fatal("Undo should succeed")
	}

	// Now redo
	redone, ok := um.Redo(restored)
	if !ok {
		t.Fatal("Redo should succeed")
	}

	if len(redone.Tracks) != 4 {
		t.Errorf("Redo restored %d tracks, want 4", len(redone.Tracks))
	}

	if redone.CursorPos != 1 {
		t.Errorf("Redo restored cursor to %d, want 1", redone.CursorPos)
	}
}

func TestUndoManager_RedoEmpty(t *testing.T) {
	um := NewUndoManager(50)

	currentState := createTestState(5, 0)
	_, ok := um.Redo(currentState)

	if ok {
		t.Error("Redo should fail on empty stack")
	}
}

func TestUndoManager_PushClearsRedo(t *testing.T) {
	um := NewUndoManager(50)

	// Create undo/redo history
	state1 := createTestState(5, 0)
	um.Push(state1)

	state2 := createTestState(4, 1)
	um.Undo(state2)

	// Verify redo stack has items
	if um.RedoSize() != 1 {
		t.Fatalf("Redo stack should have 1 item, got %d", um.RedoSize())
	}

	// Push new state - should clear redo stack
	state3 := createTestState(3, 0)
	um.Push(state3)

	if um.RedoSize() != 0 {
		t.Errorf("Push should clear redo stack, but has %d items", um.RedoSize())
	}
}

func TestUndoManager_MaxStackSize(t *testing.T) {
	um := NewUndoManager(3) // Small max size for testing

	// Push 5 states (exceeds max)
	for i := range 5 {
		state := createTestState(i+1, i)
		um.Push(state)
	}

	// Stack should be capped at 3
	if um.UndoSize() != 3 {
		t.Errorf("Undo stack size = %d, want 3 (max)", um.UndoSize())
	}

	// Oldest states should be discarded
	// We should be able to undo 3 times
	currentState := createTestState(6, 5)

	for i := range 3 {
		var ok bool

		currentState, ok = um.Undo(currentState)
		if !ok {
			t.Errorf("Undo %d failed, should have 3 items", i+1)
		}
	}

	// 4th undo should fail
	_, ok := um.Undo(currentState)
	if ok {
		t.Error("4th undo should fail (max stack size is 3)")
	}
}

func TestUndoManager_MaxRedoStackSize(t *testing.T) {
	um := NewUndoManager(3) // Small max size

	// Build up undo stack
	for i := range 5 {
		state := createTestState(i+1, i)
		um.Push(state)
	}

	// Undo multiple times to build redo stack
	currentState := createTestState(6, 5)

	for range 5 {
		var ok bool

		currentState, ok = um.Undo(currentState)
		if !ok {
			break
		}
	}

	// Redo stack should also be capped at 3
	if um.RedoSize() > 3 {
		t.Errorf("Redo stack size = %d, should be <= 3 (max)", um.RedoSize())
	}
}

func TestUndoManager_UndoRedoCycle(t *testing.T) {
	um := NewUndoManager(50)

	// Push states
	state1 := createTestState(5, 0)
	um.Push(state1)

	state2 := createTestState(4, 1)
	um.Push(state2)

	state3 := createTestState(3, 2)

	// Undo twice
	state, ok := um.Undo(state3)
	if !ok || len(state.Tracks) != 4 {
		t.Fatal("First undo failed or returned wrong state")
	}

	state, ok = um.Undo(state)
	if !ok || len(state.Tracks) != 5 {
		t.Fatal("Second undo failed or returned wrong state")
	}

	// Redo once
	state, ok = um.Redo(state)
	if !ok || len(state.Tracks) != 4 {
		t.Fatal("Redo failed or returned wrong state")
	}

	// Verify stack sizes
	if um.UndoSize() != 1 {
		t.Errorf("After undo-redo cycle, undo stack = %d, want 1", um.UndoSize())
	}

	if um.RedoSize() != 1 {
		t.Errorf("After undo-redo cycle, redo stack = %d, want 1", um.RedoSize())
	}
}

func TestUndoManager_DeepCopy(t *testing.T) {
	um := NewUndoManager(50)

	// Create state and push it
	state := createTestState(3, 0)
	originalTitle := state.Tracks[0].Title

	um.Push(state)

	// Modify the original state
	state.Tracks[0].Title = "MODIFIED"

	// Undo should return unmodified state
	currentState := createTestState(2, 1)

	restored, ok := um.Undo(currentState)
	if !ok {
		t.Fatal("Undo failed")
	}

	if restored.Tracks[0].Title != originalTitle {
		t.Errorf("State was not deep copied: got %s, want %s", restored.Tracks[0].Title, originalTitle)
	}
}

func TestUndoManager_Clear(t *testing.T) {
	um := NewUndoManager(50)

	// Build up stacks
	state1 := createTestState(5, 0)
	um.Push(state1)
	um.Push(createTestState(4, 1))

	state3 := createTestState(3, 2)
	um.Undo(state3)

	// Verify stacks have items
	if um.UndoSize() == 0 {
		t.Fatal("Undo stack should not be empty")
	}

	if um.RedoSize() == 0 {
		t.Fatal("Redo stack should not be empty")
	}

	// Clear
	um.Clear()

	if um.UndoSize() != 0 {
		t.Errorf("After clear, undo stack = %d, want 0", um.UndoSize())
	}

	if um.RedoSize() != 0 {
		t.Errorf("After clear, redo stack = %d, want 0", um.RedoSize())
	}
}

func TestUndoManager_SizeTracking(t *testing.T) {
	um := NewUndoManager(50)

	// Initially empty
	if um.UndoSize() != 0 || um.RedoSize() != 0 {
		t.Error("New manager should have empty stacks")
	}

	// Push increases undo size
	um.Push(createTestState(5, 0))

	if um.UndoSize() != 1 {
		t.Errorf("After push, undo size = %d, want 1", um.UndoSize())
	}

	// Undo decreases undo size, increases redo size
	um.Undo(createTestState(4, 1))

	if um.UndoSize() != 0 {
		t.Errorf("After undo, undo size = %d, want 0", um.UndoSize())
	}

	if um.RedoSize() != 1 {
		t.Errorf("After undo, redo size = %d, want 1", um.RedoSize())
	}
}
