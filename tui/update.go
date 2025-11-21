// ABOUTME: Event handling and state updates for the TUI
// ABOUTME: Implements the Bubble Tea Update() function and message handlers

package tui

import (
	"context"
	"runtime/debug"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"playlist-sorter/config"
)

// Update handles messages and updates the model
//
//nolint:ireturn // Bubble Tea framework requires returning tea.Model interface
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer func() {
		if r := recover(); r != nil {
			m.debugf("[PANIC] Update panic: %v", r)
			m.debugf("[PANIC] Stack trace: %s", string(debug.Stack()))
			panic(r) // Re-panic so Bubble Tea can handle it
		}
	}()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Calculate viewport dimensions
		// Right panel width: total width - left panel - padding
		viewportWidth := msg.Width - paramPanelWidth - panelPadding
		if viewportWidth < minViewportWidth {
			viewportWidth = minViewportWidth
		}

		// Height: total height minus all UI chrome (title, header, status, breakdown, help, spacing)
		viewportHeight := msg.Height - totalUIChrome
		if viewportHeight < minViewportHeight {
			viewportHeight = minViewportHeight
		}

		m.viewport.Width = viewportWidth
		m.viewport.Height = viewportHeight

		// Ensure viewport starts at top
		m.viewport.YOffset = 0
		m.ensureCursorVisible()

		// Update viewport content
		m.updateViewportContent()

		return m, nil

	case Update:
		// Ignore stale updates from old GA runs
		if msg.Epoch != m.gaEpoch {
			m.debugf("[TUI] Ignoring stale Update: epoch %d != current %d", msg.Epoch, m.gaEpoch)
			return m, waitForUpdate(m.updateChan)
		}

		// Check if track order actually changed (not just fitness)
		tracksChanged := len(m.displayedTracks) != len(msg.BestPlaylist)
		if !tracksChanged && len(m.displayedTracks) > 0 && len(msg.BestPlaylist) > 0 {
			// Check if first track changed (fast order change detection)
			if m.displayedTracks[0].Title != msg.BestPlaylist[0].Title {
				tracksChanged = true
			}
		}

		// Track improvements for time-since-improvement display
		// Only count as "improvement" if track order actually changed
		fitnessImproved := false
		if tracksChanged {
			// Track order changed - this is a real improvement
			oldFitness := m.bestFitness
			if oldFitness == 0 {
				// First update - use the initial fitness as baseline
				oldFitness = msg.BestFitness
			}

			m.lastImprovementDelta = oldFitness - msg.BestFitness
			m.previousBestFitness = oldFitness
			m.lastImprovementTime = time.Now()
			fitnessImproved = true
			m.debugf("[TUI] Tracks reordered: %.8f -> %.8f (epoch %d, gen %d)", oldFitness, msg.BestFitness, msg.Epoch, msg.Generation)
		} else if msg.BestFitness < m.bestFitness || m.bestFitness == 0 {
			// Fitness improved but order didn't change - just log it
			m.debugf("[TUI] Fitness improved but order unchanged: %.8f -> %.8f (epoch %d, gen %d)", m.bestFitness, msg.BestFitness, msg.Epoch, msg.Generation)
		}

		// Update state with GA progress
		m.bestPlaylist = msg.BestPlaylist
		m.bestFitness = msg.BestFitness
		m.breakdown = msg.Breakdown
		m.generation = msg.Generation
		m.genPerSec = msg.GenPerSec
		m.timeSinceImprovement = time.Since(m.lastImprovementTime)

		// Update m.displayedTracks with GA results (always show latest improvements)
		m.displayedTracks = msg.BestPlaylist
		m.updateViewportContent()

		if fitnessImproved {
			m.debugf("[TUI] Updated playlist display: trackCount=%d", len(m.displayedTracks))
		}

		// Auto-save the best playlist to disk (unless dry-run mode)
		if !m.dryRun && len(m.bestPlaylist) > 0 {
			if err := m.writePlaylist(m.outputPath, m.bestPlaylist); err != nil {
				m.debugf("[TUI] Auto-save FAILED: %v", err)
			} else if fitnessImproved {
				m.debugf("[TUI] Auto-saved %d tracks to %s", len(m.bestPlaylist), m.outputPath)
			}
		}

		// Queue next update
		return m, waitForUpdate(m.updateChan)

	case gaRestartMsg:
		// GA restart requested - cancel old GA and start new one
		m.cancel()
		ctx, cancel := context.WithCancel(context.Background())
		m.ctx = ctx
		m.cancel = cancel
		m.generation = 0
		m.genPerSec = 0
		m.lastImprovementTime = time.Now()
		// Note: gaEpoch already incremented in delete/undo/redo before queuing restart
		// Note: Reuse existing m.updateChan - the converter goroutine runs for the entire TUI session

		return m, tea.Batch(
			m.startGA(m.ctx, m.displayedTracks, m.gaEpoch),
			waitForUpdate(m.updateChan),
		)

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return m.handleQuitKey()

		case key.Matches(msg, keys.Tab):
			m.handleTabKey()

		case msg.Type == tea.KeyShiftUp:
			m.handleParamSelectKey(true)

		case msg.Type == tea.KeyShiftDown:
			m.handleParamSelectKey(false)

		case key.Matches(msg, keys.Up):
			m.handleUpKey()

		case key.Matches(msg, keys.Down):
			m.handleDownKey()

		case key.Matches(msg, keys.PageUp):
			m.handlePageUpKey()

		case key.Matches(msg, keys.PageDown):
			m.handlePageDownKey()

		case key.Matches(msg, keys.Home):
			m.handleHomeKey()

		case key.Matches(msg, keys.End):
			m.handleEndKey()

		case key.Matches(msg, keys.Left):
			return m, m.handleLeftKey()

		case key.Matches(msg, keys.Right):
			return m, m.handleRightKey()

		case key.Matches(msg, keys.Reset):
			return m, m.resetToDefaults()

		case key.Matches(msg, keys.Delete):
			return m, m.deleteTrack()

		case key.Matches(msg, keys.Undo):
			return m, m.undo()

		case key.Matches(msg, keys.Redo):
			return m, m.redo()
		}
	}

	return m, nil
}

// handleQuitKey handles the quit key press
func (m *model) handleQuitKey() (model, tea.Cmd) {
	m.quitting = true
	// Cancel GA context
	m.cancel()
	// Save config on quit
	if err := config.SaveConfig(m.configPath, m.sharedConfig.Get()); err != nil {
		m.debugf("[TUI] Failed to save config on quit: %v", err)
		// Continue anyway - don't block quit on config save failure
	}
	return *m, tea.Quit
}

// handleTabKey handles panel switching
func (m *model) handleTabKey() {
	if m.focusedPanel == panelParams {
		m.focusedPanel = panelPlaylist
	} else {
		m.focusedPanel = panelParams
	}
}

// handleParamSelectKey handles Shift+Up/Down for parameter selection
func (m *model) handleParamSelectKey(isUp bool) {
	if isUp {
		if m.selectedParam > 0 {
			m.selectedParam--
		}
	} else {
		if m.selectedParam < len(m.params)-1 {
			m.selectedParam++
		}
	}
}

// handleUpKey handles Up/k key press (context-aware navigation)
func (m *model) handleUpKey() {
	if m.focusedPanel == panelParams {
		// Select previous parameter
		if m.selectedParam > 0 {
			m.selectedParam--
		}
	} else {
		// Navigate tracks up
		if m.cursorPos > 0 {
			m.cursorPos--
			m.ensureCursorVisible()
			m.updateViewportContent()
		}
	}
}

// handleDownKey handles Down/j key press (context-aware navigation)
func (m *model) handleDownKey() {
	if m.focusedPanel == panelParams {
		// Select next parameter
		if m.selectedParam < len(m.params)-1 {
			m.selectedParam++
		}
	} else {
		// Navigate tracks down
		if m.cursorPos < len(m.displayedTracks)-1 {
			m.cursorPos++
			m.ensureCursorVisible()
			m.updateViewportContent()
		}
	}
}

// handlePageUpKey handles PageUp key press
func (m *model) handlePageUpKey() {
	m.cursorPos -= pageJumpSize
	if m.cursorPos < 0 {
		m.cursorPos = 0
	}
	m.ensureCursorVisible()
	m.updateViewportContent()
}

// handlePageDownKey handles PageDown key press
func (m *model) handlePageDownKey() {
	m.cursorPos += pageJumpSize
	if m.cursorPos >= len(m.displayedTracks) {
		m.cursorPos = len(m.displayedTracks) - 1
	}
	m.ensureCursorVisible()
	m.updateViewportContent()
}

// handleHomeKey handles Home/g key press
func (m *model) handleHomeKey() {
	m.cursorPos = 0
	m.ensureCursorVisible()
	m.updateViewportContent()
}

// handleEndKey handles End/G key press
func (m *model) handleEndKey() {
	if len(m.displayedTracks) > 0 {
		m.cursorPos = len(m.displayedTracks) - 1
	}
	m.ensureCursorVisible()
	m.updateViewportContent()
}

// handleLeftKey handles Left/h key press (decrease parameter when params focused)
func (m *model) handleLeftKey() tea.Cmd {
	if m.focusedPanel == panelParams {
		return m.decreaseSelectedParam()
	}
	return nil
}

// handleRightKey handles Right/l key press (increase parameter when params focused)
func (m *model) handleRightKey() tea.Cmd {
	if m.focusedPanel == panelParams {
		return m.increaseSelectedParam()
	}
	return nil
}
