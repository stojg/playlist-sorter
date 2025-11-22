// ABOUTME: Tests for ViewportManager scrolling logic
// ABOUTME: Verifies cursor-to-middle vim-style scrolling behavior

package tui

import "testing"

// getPhase is a test helper that calculates the scroll phase
func getPhase(vm *ViewportManager) ScrollPhase {
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

func TestViewportManager_TopPhase(t *testing.T) {
	// Viewport with 10 lines, 50 total items
	vm := NewViewportManager(10, 0, 50)

	tests := []struct {
		name       string
		cursorPos  int
		wantOffset int
		wantPhase  ScrollPhase
	}{
		{"cursor at 0", 0, 0, TopPhase},
		{"cursor at 1", 1, 0, TopPhase},
		{"cursor at 4 (just before middle)", 4, 0, TopPhase},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm.cursorPos = tt.cursorPos

			offset := vm.CalculateOffset()
			if offset != tt.wantOffset {
				t.Errorf("CalculateOffset() = %d, want %d", offset, tt.wantOffset)
			}

			phase := getPhase(vm)
			if phase != tt.wantPhase {
				t.Errorf("GetPhase() = %v, want %v", phase, tt.wantPhase)
			}
		})
	}
}

func TestViewportManager_MiddlePhase(t *testing.T) {
	// Viewport with 10 lines, 50 total items
	// Middle = 5, bottom threshold = 50 - 10 + 5 = 45
	vm := NewViewportManager(10, 0, 50)

	tests := []struct {
		name       string
		cursorPos  int
		wantOffset int
		wantPhase  ScrollPhase
	}{
		{"cursor at 5 (middle start)", 5, 0, MiddlePhase},
		{"cursor at 10", 10, 5, MiddlePhase},
		{"cursor at 25 (middle of list)", 25, 20, MiddlePhase},
		{"cursor at 44 (just before bottom threshold)", 44, 39, MiddlePhase},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm.cursorPos = tt.cursorPos

			offset := vm.CalculateOffset()
			if offset != tt.wantOffset {
				t.Errorf("CalculateOffset() = %d, want %d (cursor at middle should give offset = cursorPos - 5)", offset, tt.wantOffset)
			}

			phase := getPhase(vm)
			if phase != tt.wantPhase {
				t.Errorf("GetPhase() = %v, want %v", phase, tt.wantPhase)
			}
		})
	}
}

func TestViewportManager_BottomPhase(t *testing.T) {
	// Viewport with 10 lines, 50 total items
	// Bottom threshold = 50 - 10 + 5 = 45
	// Max offset = 50 - 10 = 40
	vm := NewViewportManager(10, 0, 50)

	tests := []struct {
		name       string
		cursorPos  int
		wantOffset int
		wantPhase  ScrollPhase
	}{
		{"cursor at 45 (bottom threshold)", 45, 40, BottomPhase},
		{"cursor at 48", 48, 40, BottomPhase},
		{"cursor at 49 (last item)", 49, 40, BottomPhase},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm.cursorPos = tt.cursorPos

			offset := vm.CalculateOffset()
			if offset != tt.wantOffset {
				t.Errorf("CalculateOffset() = %d, want %d", offset, tt.wantOffset)
			}

			phase := getPhase(vm)
			if phase != tt.wantPhase {
				t.Errorf("GetPhase() = %v, want %v", phase, tt.wantPhase)
			}
		})
	}
}

func TestViewportManager_SmallList(t *testing.T) {
	// Viewport with 10 lines, only 5 total items (smaller than viewport)
	vm := NewViewportManager(10, 0, 5)

	tests := []struct {
		name       string
		cursorPos  int
		wantOffset int
	}{
		{"cursor at 0", 0, 0},
		{"cursor at 2", 2, 0},
		{"cursor at 4 (last)", 4, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm.cursorPos = tt.cursorPos

			offset := vm.CalculateOffset()
			if offset != tt.wantOffset {
				t.Errorf("CalculateOffset() = %d, want %d (small list should never scroll)", offset, tt.wantOffset)
			}
		})
	}
}

func TestViewportManager_EdgeCases(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		vm := NewViewportManager(10, 0, 0)

		offset := vm.CalculateOffset()
		if offset != 0 {
			t.Errorf("Empty list should return offset 0, got %d", offset)
		}
	})

	t.Run("zero height viewport", func(t *testing.T) {
		vm := NewViewportManager(0, 5, 50)

		offset := vm.CalculateOffset()
		if offset != 0 {
			t.Errorf("Zero height viewport should return offset 0, got %d", offset)
		}
	})

	t.Run("single item list", func(t *testing.T) {
		vm := NewViewportManager(10, 0, 1)

		offset := vm.CalculateOffset()
		if offset != 0 {
			t.Errorf("Single item should return offset 0, got %d", offset)
		}
	})
}

func TestViewportManager_HeightUpdate(t *testing.T) {
	vm := NewViewportManager(10, 25, 50)

	// Initially cursor at 25 should be in middle phase with offset 20
	initialOffset := vm.CalculateOffset()
	if initialOffset != 20 {
		t.Errorf("Initial offset = %d, want 20", initialOffset)
	}

	// Update height to 20
	vm.height = 20

	// Middle now = 10, cursor at 25 should give offset 15
	newOffset := vm.CalculateOffset()
	if newOffset != 15 {
		t.Errorf("After height change, offset = %d, want 15", newOffset)
	}
}

func TestViewportManager_ExactMiddleList(t *testing.T) {
	// List with exactly viewport height items
	vm := NewViewportManager(10, 0, 10)

	tests := []struct {
		name       string
		cursorPos  int
		wantOffset int
	}{
		{"cursor at 0", 0, 0},
		{"cursor at 5 (middle)", 5, 0},
		{"cursor at 9 (last)", 9, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm.cursorPos = tt.cursorPos

			offset := vm.CalculateOffset()
			if offset != tt.wantOffset {
				t.Errorf("CalculateOffset() = %d, want %d (exact fit should never scroll)", offset, tt.wantOffset)
			}
		})
	}
}

func TestViewportManager_PhaseTransitions(t *testing.T) {
	// Test smooth transitions between phases
	vm := NewViewportManager(10, 0, 50)

	// Track offset as cursor moves from top to bottom
	var offsets []int

	for pos := range 50 {
		vm.cursorPos = pos
		offsets = append(offsets, vm.CalculateOffset())
	}

	// Verify offsets never decrease (monotonic increase or stay same)
	for i := 1; i < len(offsets); i++ {
		if offsets[i] < offsets[i-1] {
			t.Errorf("Offset decreased from %d to %d at position %d (should be monotonic)", offsets[i-1], offsets[i], i)
		}
	}

	// Verify phase 1: positions 0-4 should have offset 0
	for i := range 5 {
		if offsets[i] != 0 {
			t.Errorf("Position %d in top phase has offset %d, want 0", i, offsets[i])
		}
	}

	// Verify phase 2: positions 5-44 should have increasing offset
	for i := 5; i < 45; i++ {
		expectedOffset := i - 5
		if offsets[i] != expectedOffset {
			t.Errorf("Position %d in middle phase has offset %d, want %d", i, offsets[i], expectedOffset)
		}
	}

	// Verify phase 3: positions 45-49 should have offset 40 (maxOffset)
	for i := 45; i < 50; i++ {
		if offsets[i] != 40 {
			t.Errorf("Position %d in bottom phase has offset %d, want 40", i, offsets[i])
		}
	}
}
