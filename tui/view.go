// ABOUTME: Rendering and display functions for the TUI
// ABOUTME: Implements the Bubble Tea View() function and all render helpers

package tui

import (
	"fmt"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// View renders the TUI
func (m model) View() string {
	defer func() {
		if r := recover(); r != nil {
			m.debugf("[PANIC] View panic: %v", r)
			m.debugf("[PANIC] Stack trace: %s", string(debug.Stack()))
			panic(r) // Re-panic so Bubble Tea can handle it
		}
	}()

	if m.quitting {
		return "Saving config and exiting...\n"
	}

	// Build the UI in two columns
	leftPanel := m.renderParameters()
	rightPanel := m.renderPlaylist()

	// Create styles for the two panels with fixed widths
	// Both panels should have same height for proper horizontal joining
	// Leave room for status bar, breakdown, and help (4 lines total)
	panelHeight := m.height - (statusBarHeight + breakdownHeight + helpHeight + 1)

	leftPanelStyle := lipgloss.NewStyle().
		Width(paramPanelWidth).
		Height(panelHeight).
		Padding(0, 1)

	rightPanelWidth := m.width - paramPanelWidth - panelPadding
	if rightPanelWidth < minViewportWidth*2 {
		rightPanelWidth = minViewportWidth * 2 // Minimum width for readable track display
	}

	rightPanelStyle := lipgloss.NewStyle().
		Width(rightPanelWidth).
		Height(panelHeight).
		Padding(0, 1)

	// Combine panels horizontally
	combined := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftPanelStyle.Render(leftPanel),
		rightPanelStyle.Render(rightPanel),
	)

	// Add status bar at bottom
	statusBar := m.renderStatus()
	breakdown := m.renderBreakdown()

	return combined + "\n" + statusBar + "\n" + breakdown + "\n" + m.renderHelp()
}

// renderParameters renders the parameter control panel
func (m model) renderParameters() string {
	var s string

	title := "Algorithm parameters"
	if m.focusedPanel == panelParams {
		title = "► " + title + " [FOCUSED]"
	}

	s += titleStyle.Render(title) + "\n\n"

	for i, param := range m.params {
		var value string

		switch {
		case param.IsInt && param.IntValue != nil:
			value = strconv.Itoa(*param.IntValue)
		case !param.IsInt && param.Value != nil:
			value = fmt.Sprintf("%.2f", *param.Value)
		default:
			value = "N/A"
		}

		// Fixed width formatting to prevent column misalignment
		prefix := "  "
		if i == m.selectedParam {
			prefix = "► "
		}

		line := fmt.Sprintf("%s%-25s %6s", prefix, param.Name, value)

		if i == m.selectedParam {
			s += selectedParamStyle.Render(line) + "\n"
		} else {
			s += paramStyle.Render(line) + "\n"
		}
	}

	return s
}

// renderPlaylist renders the playlist preview with viewport scrolling
func (m model) renderPlaylist() string {
	var s string

	title := "Current best playlist"
	if m.editMode {
		title = "Playlist (EDIT MODE)"
	}

	if m.focusedPanel == panelPlaylist {
		title = "► " + title + " [FOCUSED]"
	}

	s += titleStyle.Render(title) + "\n\n"

	// Header
	header := fmt.Sprintf("%-3s %-4s %-4s %-3s %-20s %-30s %-20s %-15s",
		"#", "Key", "BPM", "Eng", "Artist", "Title", "Album", "Genre")
	s += playlistHeaderStyle.Render(header) + "\n"

	// Render viewport (content should be set in Update())
	s += m.viewport.View()

	return s
}

// updateViewportContent builds and sets the viewport content
// Renders ALL tracks - let viewport handle scrolling
func (m *model) updateViewportContent() {
	var content string

	// Render all tracks - viewport will handle scrolling via YOffset
	for i, track := range m.displayedTracks {
		artist := truncate(track.Artist, 20)
		title := truncate(track.Title, 30)
		album := truncate(track.Album, 20)
		genre := truncate(track.Genre, 15)

		line := fmt.Sprintf("%-3d %-4s %-4.0f %-3d %-20s %-30s %-20s %-15s",
			i+1,
			track.Key,
			track.BPM,
			track.Energy,
			artist,
			title,
			album,
			genre,
		)

		// Highlight cursor line
		if i == m.cursorPos {
			line = cursorStyle.Render(line)
		}

		content += line + "\n"
	}

	m.viewport.SetContent(content)
}

// renderStatus renders the status bar
func (m model) renderStatus() string {
	// Show status message if recent
	if m.statusMsg != "" && time.Since(m.statusMsgAge) < statusMessageDuration {
		return statusStyle.Width(m.width).Render(m.statusMsg)
	}

	// Format time since improvement in a readable way
	timeSince := m.timeSinceImprovement.Round(time.Second)

	// Show delta if we have improvement data
	deltaStr := ""
	if m.lastImprovementDelta != 0 {
		deltaStr = fmt.Sprintf(" | -%0.8f", m.lastImprovementDelta)
	}

	// Track info
	trackInfo := fmt.Sprintf("%d tracks | Track %d/%d",
		len(m.displayedTracks),
		m.cursorPos+1,
		len(m.displayedTracks),
	)

	// Undo/redo info
	undoInfo := fmt.Sprintf("U:%d R:%d", m.undoMgr.UndoSize(), m.undoMgr.RedoSize())

	// Edit mode flag
	editFlag := ""
	if m.editMode {
		editFlag = "[EDIT] "
	}

	status := fmt.Sprintf("%s%s | %s | Gen: %d (%.1f gen/s) | Fitness: %.8f | %s ago%s",
		editFlag,
		trackInfo,
		undoInfo,
		m.generation,
		m.genPerSec,
		m.bestFitness,
		timeSince,
		deltaStr,
	)

	return statusStyle.Width(m.width).Render(status)
}

// renderBreakdown renders the fitness breakdown showing individual components
func (m model) renderBreakdown() string {
	if m.breakdown.Total == 0 {
		// No breakdown available yet
		return ""
	}

	breakdown := fmt.Sprintf(" Harmonic: %.4f | Energy: %.4f | BPM: %.4f | Genre: %.4f | Artist: %.4f | Album: %.4f | Bias: %.4f",
		m.breakdown.Harmonic,
		m.breakdown.EnergyDelta,
		m.breakdown.BPMDelta,
		m.breakdown.GenreChange,
		m.breakdown.SameArtist,
		m.breakdown.SameAlbum,
		m.breakdown.PositionBias,
	)

	return helpStyle.Render(breakdown)
}

// renderHelp renders the help text
func (m model) renderHelp() string {
	return helpStyle.Render(" Tab: switch panel | ↑/↓/j/k: navigate | ←/→/h/l: adjust param (params panel) | Shift+↑/↓: select param | d: delete | u: undo | ctrl+r: redo | r: reset | q: quit")
}
