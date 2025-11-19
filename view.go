// ABOUTME: Read-only playlist viewer with live file watching and scrolling
// ABOUTME: Monitors playlist file for changes and displays with viewport navigation

package main

import (
	"fmt"
	"time"

	"playlist-sorter/playlist"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fsnotify/fsnotify"
)

// viewModel holds the state for the read-only playlist viewer
type viewModel struct {
	playlistPath string
	tracks       []playlist.Track
	viewport     viewport.Model
	width        int
	height       int
	fileWatcher  *fsnotify.Watcher
	lastReload   time.Time
	errorMsg     string
	ready        bool
	cursorPos    int             // Currently selected track index
	undoStack    []playlistState // Undo history (max 50)
	redoStack    []playlistState // Redo history
	modified     bool            // Tracks unsaved changes
}

// playlistState represents a snapshot of the playlist for undo/redo
type playlistState struct {
	tracks    []playlist.Track
	cursorPos int
}

// Key bindings for view mode
type viewKeyMap struct {
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Top      key.Binding
	Bottom   key.Binding
	Reload   key.Binding
	Delete   key.Binding
	Undo     key.Binding
	Redo     key.Binding
	Save     key.Binding
	Quit     key.Binding
}

var viewKeys = viewKeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("pgup", "ctrl+u"),
		key.WithHelp("pgup", "page up"),
	),
	PageDown: key.NewBinding(
		key.WithKeys("pgdown", "ctrl+d"),
		key.WithHelp("pgdn", "page down"),
	),
	Top: key.NewBinding(
		key.WithKeys("g", "home"),
		key.WithHelp("g", "go to top"),
	),
	Bottom: key.NewBinding(
		key.WithKeys("G", "end"),
		key.WithHelp("G", "go to bottom"),
	),
	Reload: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "reload"),
	),
	Delete: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "delete track"),
	),
	Undo: key.NewBinding(
		key.WithKeys("u"),
		key.WithHelp("u", "undo"),
	),
	Redo: key.NewBinding(
		key.WithKeys("ctrl+y"),
		key.WithHelp("ctrl+y", "redo"),
	),
	Save: key.NewBinding(
		key.WithKeys("w"),
		key.WithHelp("w", "save"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}

// Styles for view mode
var (
	viewTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12"))

	viewHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("10"))

	viewStatusStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("15")).
			Padding(0, 1)

	viewHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	viewErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	viewCursorStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("240")).
			Foreground(lipgloss.Color("15")).
			Bold(true)
)

// fileChangeMsg is sent when the playlist file changes
type fileChangeMsg struct{}

// reloadCompleteMsg is sent after playlist reload completes
type reloadCompleteMsg struct {
	tracks []playlist.Track
	err    error
}

// RunViewMode starts the view-only mode with file watching
func RunViewMode(playlistPath string) error {
	// Load and validate playlist using common initialization
	// View mode allows single-track playlists (just for viewing)
	tracks, err := LoadPlaylistForMode(PlaylistOptions{
		Path:    playlistPath,
		Verbose: false,
	}, true)
	if err != nil {
		return err
	}

	// Create file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}

	// Add playlist file to watcher
	if err := watcher.Add(playlistPath); err != nil {
		watcher.Close()
		return fmt.Errorf("failed to watch playlist file: %w", err)
	}

	// Create model
	m := viewModel{
		playlistPath: playlistPath,
		tracks:       tracks,
		fileWatcher:  watcher,
		lastReload:   time.Now(),
	}

	// Run program
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		watcher.Close()
		return fmt.Errorf("view mode error: %w", err)
	}

	// Cleanup
	watcher.Close()
	return nil
}

// Init initializes the view model
func (m viewModel) Init() tea.Cmd {
	return tea.Batch(
		waitForFileChange(m.fileWatcher),
		tea.EnterAltScreen,
	)
}

// waitForFileChange returns a command that waits for file system events
func waitForFileChange(watcher *fsnotify.Watcher) tea.Cmd {
	return func() tea.Msg {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return nil
				}
				// Only react to write events
				if event.Op&fsnotify.Write == fsnotify.Write {
					// Debounce: wait a bit for atomic writes to complete
					time.Sleep(100 * time.Millisecond)
					return fileChangeMsg{}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return nil
				}
				// Log error but continue watching
				debugf("[WATCHER] Error: %v", err)
			}
		}
	}
}

// reloadPlaylist loads the playlist in the background
func reloadPlaylist(path string) tea.Cmd {
	return func() tea.Msg {
		tracks, err := playlist.LoadPlaylistWithMetadata(path, false)
		if err != nil {
			return reloadCompleteMsg{err: err}
		}

		// Assign Index values
		for i := range tracks {
			tracks[i].Index = i
		}

		return reloadCompleteMsg{tracks: tracks}
	}
}

// saveState saves the current state to the undo stack
func (m *viewModel) saveState() {
	// Create snapshot of current state
	state := playlistState{
		tracks:    make([]playlist.Track, len(m.tracks)),
		cursorPos: m.cursorPos,
	}
	copy(state.tracks, m.tracks)

	// Add to undo stack
	m.undoStack = append(m.undoStack, state)

	// Limit stack to 50 entries
	if len(m.undoStack) > 50 {
		m.undoStack = m.undoStack[1:]
	}

	// Clear redo stack on new operation
	m.redoStack = nil
}

// deleteTrack deletes the track at cursor position
func (m *viewModel) deleteTrack() {
	if len(m.tracks) == 0 {
		return
	}

	// Save state before delete
	m.saveState()

	// Remove track at cursor position
	m.tracks = append(m.tracks[:m.cursorPos], m.tracks[m.cursorPos+1:]...)

	// Adjust cursor if we deleted the last track
	if m.cursorPos >= len(m.tracks) && len(m.tracks) > 0 {
		m.cursorPos = len(m.tracks) - 1
	}
	if len(m.tracks) == 0 {
		m.cursorPos = 0
	}

	// Mark as modified
	m.modified = true

	// Update viewport content
	m.viewport.SetContent(m.renderPlaylistContent())
}

// undo restores the previous state
func (m *viewModel) undo() {
	if len(m.undoStack) == 0 {
		return
	}

	// Save current state to redo stack
	redoState := playlistState{
		tracks:    make([]playlist.Track, len(m.tracks)),
		cursorPos: m.cursorPos,
	}
	copy(redoState.tracks, m.tracks)
	m.redoStack = append(m.redoStack, redoState)

	// Pop from undo stack
	lastIdx := len(m.undoStack) - 1
	state := m.undoStack[lastIdx]
	m.undoStack = m.undoStack[:lastIdx]

	// Restore state
	m.tracks = make([]playlist.Track, len(state.tracks))
	copy(m.tracks, state.tracks)
	m.cursorPos = state.cursorPos

	// Still modified if undo stack not empty
	m.modified = len(m.undoStack) > 0

	// Update viewport content
	m.viewport.SetContent(m.renderPlaylistContent())
}

// redo restores a previously undone state
func (m *viewModel) redo() {
	if len(m.redoStack) == 0 {
		return
	}

	// Save current state to undo stack
	m.saveState()

	// Pop from redo stack
	lastIdx := len(m.redoStack) - 1
	state := m.redoStack[lastIdx]
	m.redoStack = m.redoStack[:lastIdx]

	// Restore state
	m.tracks = make([]playlist.Track, len(state.tracks))
	copy(m.tracks, state.tracks)
	m.cursorPos = state.cursorPos

	// Mark as modified
	m.modified = true

	// Update viewport content
	m.viewport.SetContent(m.renderPlaylistContent())
}

// savePlaylist writes the current playlist to disk
func (m *viewModel) savePlaylist() error {
	if err := playlist.WritePlaylist(m.playlistPath, m.tracks); err != nil {
		return err
	}

	// Clear modified flag and undo/redo stacks
	m.modified = false
	m.undoStack = nil
	m.redoStack = nil

	return nil
}

// ensureCursorVisible scrolls viewport to keep cursor in view
func (m *viewModel) ensureCursorVisible() {
	// Get viewport bounds
	viewportTop := m.viewport.YOffset
	viewportBottom := m.viewport.YOffset + m.viewport.Height - 1

	// Scroll if cursor is out of view
	if m.cursorPos < viewportTop {
		m.viewport.SetYOffset(m.cursorPos)
	} else if m.cursorPos > viewportBottom {
		m.viewport.SetYOffset(m.cursorPos - m.viewport.Height + 1)
	}
}

// Update handles messages and updates the model
func (m viewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		if !m.ready {
			// Initialize viewport on first size message
			headerHeight := 3 // Title + header row + separator
			footerHeight := 2 // Status + help
			m.viewport = viewport.New(msg.Width, msg.Height-headerHeight-footerHeight)
			m.viewport.SetContent(m.renderPlaylistContent())
			m.ready = true
		} else {
			// Update viewport size
			headerHeight := 3
			footerHeight := 2
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - headerHeight - footerHeight
		}

		return m, nil

	case fileChangeMsg:
		// File changed, reload playlist
		return m, tea.Batch(
			reloadPlaylist(m.playlistPath),
			waitForFileChange(m.fileWatcher), // Continue watching
		)

	case reloadCompleteMsg:
		if msg.err != nil {
			m.errorMsg = fmt.Sprintf("Error reloading: %v", msg.err)
		} else {
			m.tracks = msg.tracks
			m.lastReload = time.Now()
			m.errorMsg = ""
			// Update viewport content
			m.viewport.SetContent(m.renderPlaylistContent())
		}
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, viewKeys.Quit):
			// Check for unsaved changes
			if m.modified {
				// Check if we already showed the warning (second q press)
				if m.errorMsg == "Unsaved changes! Press 'w' to save or 'q' again to quit without saving" {
					// Second q press, force quit
					return m, tea.Quit
				}
				// First q press, show warning
				m.errorMsg = "Unsaved changes! Press 'w' to save or 'q' again to quit without saving"
				return m, nil
			}
			return m, tea.Quit

		case key.Matches(msg, viewKeys.Up):
			if m.cursorPos > 0 {
				m.cursorPos--
				m.ensureCursorVisible()
				m.viewport.SetContent(m.renderPlaylistContent())
			}

		case key.Matches(msg, viewKeys.Down):
			if m.cursorPos < len(m.tracks)-1 {
				m.cursorPos++
				m.ensureCursorVisible()
				m.viewport.SetContent(m.renderPlaylistContent())
			}

		case key.Matches(msg, viewKeys.PageUp):
			m.cursorPos -= m.viewport.Height
			if m.cursorPos < 0 {
				m.cursorPos = 0
			}
			m.ensureCursorVisible()
			m.viewport.SetContent(m.renderPlaylistContent())

		case key.Matches(msg, viewKeys.PageDown):
			m.cursorPos += m.viewport.Height
			if m.cursorPos >= len(m.tracks) {
				m.cursorPos = len(m.tracks) - 1
			}
			if m.cursorPos < 0 {
				m.cursorPos = 0
			}
			m.ensureCursorVisible()
			m.viewport.SetContent(m.renderPlaylistContent())

		case key.Matches(msg, viewKeys.Top):
			m.cursorPos = 0
			m.viewport.GotoTop()
			m.viewport.SetContent(m.renderPlaylistContent())

		case key.Matches(msg, viewKeys.Bottom):
			if len(m.tracks) > 0 {
				m.cursorPos = len(m.tracks) - 1
			}
			m.viewport.GotoBottom()
			m.viewport.SetContent(m.renderPlaylistContent())

		case key.Matches(msg, viewKeys.Reload):
			return m, reloadPlaylist(m.playlistPath)

		case key.Matches(msg, viewKeys.Delete):
			m.deleteTrack()
			m.ensureCursorVisible()

		case key.Matches(msg, viewKeys.Undo):
			m.undo()
			m.ensureCursorVisible()

		case key.Matches(msg, viewKeys.Redo):
			m.redo()
			m.ensureCursorVisible()

		case key.Matches(msg, viewKeys.Save):
			if err := m.savePlaylist(); err != nil {
				m.errorMsg = fmt.Sprintf("Save failed: %v", err)
			} else {
				m.errorMsg = "Saved successfully"
			}
		}
	}

	// Update viewport
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View renders the view
func (m viewModel) View() string {
	if !m.ready {
		return "Loading..."
	}

	// Title
	title := viewTitleStyle.Render(fmt.Sprintf("Playlist Viewer: %s", m.playlistPath))

	// Header row
	header := viewHeaderStyle.Render(fmt.Sprintf("%-3s %-4s %-4s %-3s %-20s %-30s %-20s %-15s",
		"#", "Key", "BPM", "Eng", "Artist", "Title", "Album", "Genre"))

	// Viewport with playlist
	viewportContent := m.viewport.View()

	// Status bar
	status := m.renderStatus()

	// Help text
	help := m.renderHelp()

	return fmt.Sprintf("%s\n%s\n%s\n%s\n%s", title, header, viewportContent, status, help)
}

// renderPlaylistContent renders the full playlist for the viewport
func (m viewModel) renderPlaylistContent() string {
	var content string

	for i, track := range m.tracks {
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
			line = viewCursorStyle.Render(line)
		}

		if i < len(m.tracks)-1 {
			content += line + "\n"
		} else {
			content += line // No trailing newline on last track
		}
	}

	return content
}

// renderStatus renders the status bar
func (m viewModel) renderStatus() string {
	reloadTime := m.lastReload.Format("15:04:05")

	// Build status parts
	modifiedIndicator := ""
	if m.modified {
		modifiedIndicator = viewErrorStyle.Render("[MODIFIED]") + " | "
	}

	undoRedoInfo := ""
	if len(m.undoStack) > 0 || len(m.redoStack) > 0 {
		undoRedoInfo = fmt.Sprintf(" | u:%d r:%d", len(m.undoStack), len(m.redoStack))
	}

	var statusText string
	if m.errorMsg != "" {
		statusText = fmt.Sprintf("%s%d tracks | Cursor: %d | %s%s",
			modifiedIndicator,
			len(m.tracks),
			m.cursorPos+1,
			viewErrorStyle.Render(m.errorMsg),
			undoRedoInfo,
		)
	} else {
		statusText = fmt.Sprintf("%s%d tracks | Cursor: %d | Last reload: %s%s",
			modifiedIndicator,
			len(m.tracks),
			m.cursorPos+1,
			reloadTime,
			undoRedoInfo,
		)
	}

	return viewStatusStyle.Width(m.width).Render(statusText)
}

// renderHelp renders the help text
func (m viewModel) renderHelp() string {
	return viewHelpStyle.Render("↑/↓: move cursor | d: delete | u: undo | ctrl+y: redo | w: save | r: reload | q: quit")
}
