// ABOUTME: Undo/redo stack manager for playlist editing
// ABOUTME: Manages state history with maximum stack size limit

package tui

import "playlist-sorter/playlist"

// PlaylistState captures a snapshot of the playlist for undo/redo
type PlaylistState struct {
	Tracks    []playlist.Track
	CursorPos int
}

// UndoManager manages undo/redo stacks with maximum size limit
type UndoManager struct {
	undoStack []PlaylistState
	redoStack []PlaylistState
	maxSize   int
}

// NewUndoManager creates a new undo manager with the specified max stack size
func NewUndoManager(maxSize int) *UndoManager {
	return &UndoManager{
		undoStack: []PlaylistState{},
		redoStack: []PlaylistState{},
		maxSize:   maxSize,
	}
}

// Push saves a new state to the undo stack
// Clears the redo stack (you can't redo after a new action)
func (um *UndoManager) Push(state PlaylistState) {
	// Make a deep copy of tracks
	stateCopy := PlaylistState{
		Tracks:    append([]playlist.Track{}, state.Tracks...),
		CursorPos: state.CursorPos,
	}

	um.undoStack = append(um.undoStack, stateCopy)

	// Enforce max size
	if len(um.undoStack) > um.maxSize {
		um.undoStack = um.undoStack[1:]
	}

	// Clear redo stack on new edit
	um.redoStack = []PlaylistState{}
}

// Undo restores the previous state
// Returns the state and true if undo was successful, or zero value and false if nothing to undo
func (um *UndoManager) Undo(currentState PlaylistState) (PlaylistState, bool) {
	if len(um.undoStack) == 0 {
		return PlaylistState{}, false
	}

	// Save current state to redo stack
	redoState := PlaylistState{
		Tracks:    append([]playlist.Track{}, currentState.Tracks...),
		CursorPos: currentState.CursorPos,
	}

	um.redoStack = append(um.redoStack, redoState)

	// Enforce max size on redo stack
	if len(um.redoStack) > um.maxSize {
		um.redoStack = um.redoStack[1:]
	}

	// Pop from undo stack
	state := um.undoStack[len(um.undoStack)-1]
	um.undoStack = um.undoStack[:len(um.undoStack)-1]

	return state, true
}

// Redo restores the next state
// Returns the state and true if redo was successful, or zero value and false if nothing to redo
func (um *UndoManager) Redo(currentState PlaylistState) (PlaylistState, bool) {
	if len(um.redoStack) == 0 {
		return PlaylistState{}, false
	}

	// Save current state to undo stack
	undoState := PlaylistState{
		Tracks:    append([]playlist.Track{}, currentState.Tracks...),
		CursorPos: currentState.CursorPos,
	}

	um.undoStack = append(um.undoStack, undoState)

	// Enforce max size on undo stack
	if len(um.undoStack) > um.maxSize {
		um.undoStack = um.undoStack[1:]
	}

	// Pop from redo stack
	state := um.redoStack[len(um.redoStack)-1]
	um.redoStack = um.redoStack[:len(um.redoStack)-1]

	return state, true
}

// UndoSize returns the number of items in the undo stack
func (um *UndoManager) UndoSize() int {
	return len(um.undoStack)
}

// RedoSize returns the number of items in the redo stack
func (um *UndoManager) RedoSize() int {
	return len(um.redoStack)
}

// Clear clears both stacks
func (um *UndoManager) Clear() {
	um.undoStack = []PlaylistState{}
	um.redoStack = []PlaylistState{}
}
