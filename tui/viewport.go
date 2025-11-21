// ABOUTME: Viewport manager for cursor-to-middle scrolling
// ABOUTME: Implements vim/less style viewport scrolling behavior

package tui

// ViewportManager handles cursor visibility and viewport scrolling
// Implements vim/less style scrolling: cursor moves to middle, then content scrolls
type ViewportManager struct {
	height     int // Viewport height in lines
	cursorPos  int // Current cursor position
	totalItems int // Total number of items
}

// NewViewportManager creates a new viewport manager
func NewViewportManager(height, cursorPos, totalItems int) *ViewportManager {
	return &ViewportManager{
		height:     height,
		cursorPos:  cursorPos,
		totalItems: totalItems,
	}
}

// SetHeight updates the viewport height
func (vm *ViewportManager) SetHeight(height int) {
	vm.height = height
}

// SetCursorPos updates the cursor position
func (vm *ViewportManager) SetCursorPos(pos int) {
	vm.cursorPos = pos
}

// SetTotalItems updates the total item count
func (vm *ViewportManager) SetTotalItems(total int) {
	vm.totalItems = total
}

// CalculateOffset computes the viewport Y offset to keep cursor visible
// Returns the offset value that should be applied to the viewport
//
// Scrolling behavior:
// - Phase 1 (top): Cursor moves freely, viewport stays at 0
// - Phase 2 (middle): Cursor stays at middle, content scrolls
// - Phase 3 (bottom): Viewport shows end, cursor moves to bottom
func (vm *ViewportManager) CalculateOffset() int {
	if vm.totalItems == 0 || vm.height < 1 {
		return 0
	}

	middle := vm.height / 2

	// Phase 1: Cursor in top half - cursor moves, viewport stays at top
	if vm.cursorPos < middle {
		return 0
	}

	// Phase 2: Cursor in middle section - cursor stays at middle, content scrolls
	// This continues until we're close to the bottom
	bottomThreshold := vm.totalItems - vm.height + middle
	if vm.cursorPos < bottomThreshold {
		// Keep cursor at middle of viewport
		return vm.cursorPos - middle
	}

	// Phase 3: Near bottom - viewport shows end, cursor moves down
	// Set viewport to show the last viewportHeight items
	maxOffset := vm.totalItems - vm.height
	if maxOffset < 0 {
		maxOffset = 0
	}

	return maxOffset
}

// ScrollPhase returns which scrolling phase the cursor is currently in
type ScrollPhase int

const (
	TopPhase    ScrollPhase = iota // Cursor moves, viewport at top
	MiddlePhase                    // Cursor at middle, content scrolls
	BottomPhase                    // Viewport at bottom, cursor moves
)

// GetPhase returns the current scrolling phase
func (vm *ViewportManager) GetPhase() ScrollPhase {
	if vm.totalItems == 0 || vm.height < 1 {
		return TopPhase
	}
	middle := vm.height / 2
	if vm.cursorPos < middle {
		return TopPhase
	}
	bottomThreshold := vm.totalItems - vm.height + middle
	if vm.cursorPos < bottomThreshold {
		return MiddlePhase
	}
	return BottomPhase
}
