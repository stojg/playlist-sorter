// ABOUTME: Terminal UI model and core state management
// ABOUTME: Bubble Tea model implementation with GA integration

package tui

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"playlist-sorter/config"
	"playlist-sorter/playlist"
)

// Panel identifiers
const (
	panelParams   = "params"
	panelPlaylist = "playlist"
)

// Parameter represents a tunable GA parameter with constraints
type Parameter struct {
	Name     string
	Value    *float64 // Pointer to actual config field
	IntValue *int     // For integer parameters
	Min      float64
	Max      float64
	Step     float64
	IsInt    bool
}

// gaRestartMsg signals that GA should restart with new tracks
type gaRestartMsg struct{}

// model holds the TUI state
type model struct {
	// Dependencies (injected interfaces)
	configProvider ConfigProvider
	gaRunner       GARunner
	playlistWriter PlaylistWriter
	logger         Logger

	// Configuration
	localConfig *config.GAConfig // Local config that params point to (pointer so addresses stay valid)
	paramMgr    *ParamManager    // Parameter manager
	configPath  string           // Config file path

	// GA state
	bestPlaylist         []playlist.Track // Best playlist from GA
	originalTracks       []playlist.Track // Original tracks (for restart in Phase 5)
	bestFitness          float64          // Current best fitness
	previousBestFitness  float64          // Fitness at last improvement (for delta calculation)
	lastImprovementDelta float64          // Fitness improvement amount from last improvement
	breakdown            Breakdown        // Fitness breakdown
	generation           int              // Current generation
	genPerSec            float64          // Generations per second
	lastImprovementTime  time.Time        // Time of last fitness improvement
	timeSinceImprovement time.Duration    // Duration since last improvement

	// GA lifecycle
	ctx        context.Context    //nolint:containedctx // Bubble Tea needs context for cancellation
	cancel     context.CancelFunc // Cancel function
	updateChan chan Update        // Channel for GA updates
	gaEpoch    int                // Increments each GA restart to track stale updates

	// File I/O
	playlistPath string // Playlist file path for reading
	outputPath   string // Output path for saving (may differ from playlistPath)
	dryRun       bool   // If true, don't save changes

	// UI state
	width        int
	height       int
	quitting     bool
	statusMsg    string    // Temporary status message (e.g., "Playlist saved")
	statusMsgAge time.Time // When status message was set
	focusedPanel string    // "params" or "playlist" - which panel has focus

	// Track browsing and editing
	cursorPos int              // Current cursor position in track list
	viewport  viewport.Model   // Viewport for scrolling track list
	undoMgr   *UndoManager     // Undo/redo history manager
	editMode  bool             // True when user is manually editing (GA paused)
	tracks    []playlist.Track // Current tracks (either from GA or manual edits)
}

// Key bindings
type keyMap struct {
	Up    key.Binding
	Down  key.Binding
	Left  key.Binding
	Right key.Binding
	Reset key.Binding
	Quit  key.Binding
	// Track navigation
	PageUp   key.Binding
	PageDown key.Binding
	Home     key.Binding
	End      key.Binding
	// Track editing
	Delete key.Binding
	Undo   key.Binding
	Redo   key.Binding
	// Panel switching
	Tab key.Binding
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "navigate"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "navigate"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "decrease param"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "increase param"),
	),
	Reset: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "reset params"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("pgup"),
		key.WithHelp("pgup", "page up"),
	),
	PageDown: key.NewBinding(
		key.WithKeys("pgdown"),
		key.WithHelp("pgdn", "page down"),
	),
	Home: key.NewBinding(
		key.WithKeys("home", "g"),
		key.WithHelp("home/g", "first track"),
	),
	End: key.NewBinding(
		key.WithKeys("end", "G"),
		key.WithHelp("end/G", "last track"),
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
		key.WithKeys("ctrl+r"),
		key.WithHelp("ctrl+r", "redo"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch panel"),
	),
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12"))

	paramStyle = lipgloss.NewStyle().
			Padding(0, 1)

	selectedParamStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("240")).
				Foreground(lipgloss.Color("15")).
				Bold(true).
				Padding(0, 1)

	playlistHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("10"))

	statusStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("15")).
			Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	cursorStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("240")).
			Foreground(lipgloss.Color("15"))
)

// Run starts the TUI mode with injected dependencies
func Run(opts Options, deps Dependencies) error {
	// Load and validate playlist
	tracks, err := deps.PlaylistLoader.Load(opts.PlaylistPath, true)
	if err != nil {
		return err
	}

	// Create model with injected dependencies
	m := initModel(tracks, opts, deps)

	// Run program
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	// Save the optimized playlist on exit (unless dry-run mode)
	if m, ok := finalModel.(model); ok && len(m.bestPlaylist) > 0 {
		if m.dryRun {
			fmt.Println("\n--dry-run mode: playlist not modified")
		} else {
			if err := deps.PlaylistWriter.Write(m.outputPath, m.bestPlaylist); err != nil {
				return fmt.Errorf("failed to save playlist: %w", err)
			}

			fmt.Printf("\nSaved optimized playlist to: %s\n", m.outputPath)
		}
	}

	return nil
}

// initModel creates the initial model with injected dependencies
func initModel(tracks []playlist.Track, opts Options, deps Dependencies) model {
	// Get config from provider
	cfg := deps.ConfigProvider.Get()

	// Allocate localConfig on heap so pointers remain valid
	localConfig := &cfg

	// Create context for GA cancellation
	ctx, cancel := context.WithCancel(context.Background())

	// Determine output path
	outputPath := opts.PlaylistPath
	if opts.OutputPath != "" {
		outputPath = opts.OutputPath
	}

	m := model{
		// Injected dependencies
		configProvider: deps.ConfigProvider,
		gaRunner:       deps.GARunner,
		playlistWriter: deps.PlaylistWriter,
		logger:         deps.Logger,

		// Configuration
		localConfig: localConfig,
		configPath:  deps.ConfigPath,

		// GA state
		bestPlaylist:        tracks, // Start with original order
		originalTracks:      tracks,
		lastImprovementTime: time.Now(),

		// GA lifecycle
		ctx:        ctx,
		cancel:     cancel,
		updateChan: make(chan Update, 10),
		gaEpoch:    0,

		// File I/O
		playlistPath: opts.PlaylistPath,
		outputPath:   outputPath,
		dryRun:       opts.DryRun,

		// UI state
		viewport:     viewport.New(0, 0), // Width and height set on first WindowSizeMsg
		focusedPanel: panelPlaylist,

		// Track editing
		cursorPos: 0,
		tracks:    tracks,
		undoMgr:   NewUndoManager(50), // Max 50 undo/redo items
		editMode:  false,
	}

	// Build parameter list with pointers to localConfig fields
	// All fitness weights now use [0,1] range due to component normalization
	params := []Parameter{
		{"Harmonic Weight", &localConfig.HarmonicWeight, nil, 0, 1, 0.01, false},
		{"Energy Delta Weight", &localConfig.EnergyDeltaWeight, nil, 0, 1, 0.01, false},
		{"BPM Delta Weight", &localConfig.BPMDeltaWeight, nil, 0, 1, 0.01, false},
		{"Genre Weight", &localConfig.GenreWeight, nil, -1, 1, 0.01, false},
		{"Same Artist Penalty", &localConfig.SameArtistPenalty, nil, 0, 1, 0.01, false},
		{"Same Album Penalty", &localConfig.SameAlbumPenalty, nil, 0, 1, 0.01, false},
		{"Low Energy Bias Portion", &localConfig.LowEnergyBiasPortion, nil, 0, 1, 0.01, false},
		{"Low Energy Bias Weight", &localConfig.LowEnergyBiasWeight, nil, 0, 1, 0.01, false},
	}
	m.paramMgr = NewParamManager(params)

	return m
}

// Init initializes the model
func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.runGA(m.ctx, m.originalTracks, m.gaEpoch),
		waitForUpdate(m.updateChan),
		tea.EnterAltScreen,
	)
}

// Update handles messages and updates the model
//
//nolint:ireturn // Bubble Tea framework requires returning tea.Model interface
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer func() {
		if r := recover(); r != nil {
			m.logger.Debugf("[PANIC] Update panic: %v", r)
			m.logger.Debugf("[PANIC] Stack trace: %s", string(debug.Stack()))
			panic(r) // Re-panic so Bubble Tea can handle it
		}
	}()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Calculate viewport dimensions
		// Right panel width: total width - left panel (45) - padding (2)
		viewportWidth := msg.Width - 47
		if viewportWidth < 20 {
			viewportWidth = 20 // Minimum width
		}

		// Height: total - title(2) - header(1) - status(1) - breakdown(1) - help(1) - spacing(2) = height - 8
		viewportHeight := msg.Height - 8
		if viewportHeight < 5 {
			viewportHeight = 5 // Minimum height
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
			m.logger.Debugf("[TUI] Ignoring stale Update: epoch %d != current %d", msg.Epoch, m.gaEpoch)
			return m, waitForUpdate(m.updateChan)
		}

		// Check if track order actually changed (not just fitness)
		tracksChanged := len(m.tracks) != len(msg.BestPlaylist)
		if !tracksChanged && len(m.tracks) > 0 && len(msg.BestPlaylist) > 0 {
			// Check if first track changed (fast order change detection)
			if m.tracks[0].Title != msg.BestPlaylist[0].Title {
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
			m.logger.Debugf("[TUI] Tracks reordered: %.8f -> %.8f (epoch %d, gen %d)", oldFitness, msg.BestFitness, msg.Epoch, msg.Generation)
		} else if msg.BestFitness < m.bestFitness || m.bestFitness == 0 {
			// Fitness improved but order didn't change - just log it
			m.logger.Debugf("[TUI] Fitness improved but order unchanged: %.8f -> %.8f (epoch %d, gen %d)", m.bestFitness, msg.BestFitness, msg.Epoch, msg.Generation)
		}

		// Update state with GA progress
		m.bestPlaylist = msg.BestPlaylist
		m.bestFitness = msg.BestFitness
		m.breakdown = msg.Breakdown
		m.generation = msg.Generation
		m.genPerSec = msg.GenPerSec
		m.timeSinceImprovement = time.Since(m.lastImprovementTime)

		// Update m.tracks with GA results (always show latest improvements)
		m.tracks = msg.BestPlaylist
		m.updateViewportContent()

		if fitnessImproved {
			m.logger.Debugf("[TUI] Updated playlist display: trackCount=%d", len(m.tracks))
		}

		// Auto-save the best playlist to disk (unless dry-run mode)
		if !m.dryRun && len(m.bestPlaylist) > 0 {
			if err := m.playlistWriter.Write(m.outputPath, m.bestPlaylist); err != nil {
				m.logger.Debugf("[TUI] Auto-save FAILED: %v", err)
			} else if fitnessImproved {
				m.logger.Debugf("[TUI] Auto-saved %d tracks to %s", len(m.bestPlaylist), m.outputPath)
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
		m.updateChan = make(chan Update, 10)
		// Note: gaEpoch already incremented in delete/undo/redo before queuing restart

		return m, tea.Batch(
			m.runGA(m.ctx, m.tracks, m.gaEpoch),
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
	_ = config.SaveConfig(m.configPath, m.configProvider.Get())
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
		m.paramMgr.SelectPrevious()
	} else {
		m.paramMgr.SelectNext()
	}
}

// handleUpKey handles Up/k key press (context-aware navigation)
func (m *model) handleUpKey() {
	if m.focusedPanel == panelParams {
		// Select previous parameter
		m.paramMgr.SelectPrevious()
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
		m.paramMgr.SelectNext()
	} else {
		// Navigate tracks down
		if m.cursorPos < len(m.tracks)-1 {
			m.cursorPos++
			m.ensureCursorVisible()
			m.updateViewportContent()
		}
	}
}

// handlePageUpKey handles PageUp key press
func (m *model) handlePageUpKey() {
	m.cursorPos -= 10
	if m.cursorPos < 0 {
		m.cursorPos = 0
	}
	m.ensureCursorVisible()
	m.updateViewportContent()
}

// handlePageDownKey handles PageDown key press
func (m *model) handlePageDownKey() {
	m.cursorPos += 10
	if m.cursorPos >= len(m.tracks) {
		m.cursorPos = len(m.tracks) - 1
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
	if len(m.tracks) > 0 {
		m.cursorPos = len(m.tracks) - 1
	}
	m.ensureCursorVisible()
	m.updateViewportContent()
}

// handleLeftKey handles Left/h key press (decrease parameter when params focused)
func (m *model) handleLeftKey() tea.Cmd {
	if m.focusedPanel == panelParams {
		return m.decreaseParam()
	}
	return nil
}

// handleRightKey handles Right/l key press (increase parameter when params focused)
func (m *model) handleRightKey() tea.Cmd {
	if m.focusedPanel == panelParams {
		return m.increaseParam()
	}
	return nil
}

// View renders the TUI
func (m model) View() string {
	defer func() {
		if r := recover(); r != nil {
			m.logger.Debugf("[PANIC] View panic: %v", r)
			m.logger.Debugf("[PANIC] Stack trace: %s", string(debug.Stack()))
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
	panelHeight := m.height - 4 // Leave room for status bar, breakdown, help

	leftPanelStyle := lipgloss.NewStyle().
		Width(45).
		Height(panelHeight).
		Padding(0, 1)

	rightPanelWidth := m.width - 47
	if rightPanelWidth < 40 {
		rightPanelWidth = 40 // Minimum width for track display
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

// runGA starts the GA in a goroutine and returns a command
// runGA starts the GA using the injected GARunner interface
func (m *model) runGA(ctx context.Context, tracks []playlist.Track, epoch int) tea.Cmd {
	return func() tea.Msg {
		defer func() {
			if r := recover(); r != nil {
				m.logger.Debugf("[PANIC] runGA panic: %v", r)
				m.logger.Debugf("[PANIC] Stack trace: %s", string(debug.Stack()))
				panic(r) // Re-panic after logging
			}
		}()

		// Run GA via interface (blocks until context cancelled or GA completes)
		m.gaRunner.Run(ctx, tracks, m.configProvider, m.updateChan, epoch)
		return nil
	}
}

// waitForUpdate waits for GA updates and returns them as messages
func waitForUpdate(updateChan <-chan Update) tea.Cmd {
	return func() tea.Msg {
		update, ok := <-updateChan
		if !ok {
			// Channel closed
			return nil
		}

		return update
	}
}

// increaseParam increases the selected parameter value and restarts GA
func (m *model) increaseParam() tea.Cmd {
	if m.paramMgr.Increase() {
		return m.syncConfigToGA()
	}
	return nil
}

// decreaseParam decreases the selected parameter value and restarts GA
func (m *model) decreaseParam() tea.Cmd {
	if m.paramMgr.Decrease() {
		return m.syncConfigToGA()
	}
	return nil
}

// resetToDefaults resets all parameters to their default values and restarts GA
func (m *model) resetToDefaults() tea.Cmd {
	defaults := config.DefaultConfig()
	m.paramMgr.ResetToDefaults(defaults)
	return m.syncConfigToGA()
}

// syncConfigToGA syncs parameter values to the shared config and restarts GA
// Returns command to restart GA with new weights
func (m *model) syncConfigToGA() tea.Cmd {
	// Parameters already modified m.localConfig directly via pointers
	// Just copy the entire struct to shared config (thread-safe)
	selected := m.paramMgr.GetSelected()
	if selected != nil {
		var value float64
		if selected.IsInt {
			value = float64(*selected.IntValue)
		} else {
			value = *selected.Value
		}
		m.logger.Debugf("[TUI] Parameter changed - %s: %.2f (Harmonic: %.2f, Energy: %.2f, BPM: %.2f)",
			selected.Name,
			value,
			m.localConfig.HarmonicWeight,
			m.localConfig.EnergyDeltaWeight,
			m.localConfig.BPMDeltaWeight)
	}
	m.configProvider.Update(*m.localConfig)

	// Increment epoch immediately to invalidate any pending GA updates with old weights
	m.gaEpoch++

	m.logger.Debugf("[TUI] Config synced - restarting GA with epoch %d for new weights", m.gaEpoch)

	// Restart GA with new weights (same tracks, new epoch)
	return m.restartGA()
}

// ensureCursorVisible adjusts viewport offset to keep cursor visible with middle-of-screen scrolling
// Implements vim/less style scrolling using ViewportManager
func (m *model) ensureCursorVisible() {
	vm := NewViewportManager(m.viewport.Height, m.cursorPos, len(m.tracks))
	offset := vm.CalculateOffset()
	m.viewport.SetYOffset(offset)
}

// pushUndo saves current state to undo stack using UndoManager
func (m *model) pushUndo() {
	state := PlaylistState{
		Tracks:    m.tracks,
		CursorPos: m.cursorPos,
	}
	m.undoMgr.Push(state)
}

// deleteTrack removes the track at cursor position and restarts GA
func (m *model) deleteTrack() tea.Cmd {
	if len(m.tracks) == 0 {
		return nil
	}

	// Save current state to undo stack
	m.pushUndo()

	// Remove track at cursor
	m.tracks = append(m.tracks[:m.cursorPos], m.tracks[m.cursorPos+1:]...)

	// Set edit mode
	m.editMode = true

	// Increment epoch immediately to invalidate any pending GA updates
	m.gaEpoch++

	// Adjust cursor if needed
	if m.cursorPos >= len(m.tracks) && len(m.tracks) > 0 {
		m.cursorPos = len(m.tracks) - 1
	}

	// Update status message
	m.statusMsg = fmt.Sprintf("Deleted track (Undo: %d, Redo: %d)", m.undoMgr.UndoSize(), m.undoMgr.RedoSize())
	m.statusMsgAge = time.Now()

	// Update viewport
	m.updateViewportContent()

	// Auto-save the edited playlist
	m.autoSave()

	// Restart GA with edited track list
	return m.restartGA()
}

// undo restores previous state from undo stack using UndoManager
func (m *model) undo() tea.Cmd {
	currentState := PlaylistState{
		Tracks:    m.tracks,
		CursorPos: m.cursorPos,
	}

	state, ok := m.undoMgr.Undo(currentState)
	if !ok {
		m.statusMsg = "Nothing to undo"
		m.statusMsgAge = time.Now()
		return nil
	}

	// Restore state
	m.tracks = state.Tracks
	m.cursorPos = state.CursorPos
	m.ensureCursorVisible()

	// Increment epoch immediately to invalidate any pending GA updates
	m.gaEpoch++

	// Update status message
	m.statusMsg = fmt.Sprintf("Undo (Undo: %d, Redo: %d)", m.undoMgr.UndoSize(), m.undoMgr.RedoSize())
	m.statusMsgAge = time.Now()

	// Update viewport
	m.updateViewportContent()

	// Auto-save the restored playlist
	m.autoSave()

	// Restart GA with restored tracks
	return m.restartGA()
}

// redo restores next state from redo stack using UndoManager
func (m *model) redo() tea.Cmd {
	currentState := PlaylistState{
		Tracks:    m.tracks,
		CursorPos: m.cursorPos,
	}

	state, ok := m.undoMgr.Redo(currentState)
	if !ok {
		m.statusMsg = "Nothing to redo"
		m.statusMsgAge = time.Now()
		return nil
	}

	// Restore state
	m.tracks = state.Tracks
	m.cursorPos = state.CursorPos
	m.ensureCursorVisible()

	// Increment epoch immediately to invalidate any pending GA updates
	m.gaEpoch++

	// Update status message
	m.statusMsg = fmt.Sprintf("Redo (Undo: %d, Redo: %d)", m.undoMgr.UndoSize(), m.undoMgr.RedoSize())
	m.statusMsgAge = time.Now()

	// Update viewport
	m.updateViewportContent()

	// Auto-save the restored playlist
	m.autoSave()

	// Restart GA with restored tracks
	return m.restartGA()
}

// autoSave writes current tracks to disk (silent, no status messages)
func (m *model) autoSave() {
	if m.dryRun {
		return
	}

	_ = m.playlistWriter.Write(m.outputPath, m.tracks)
}

// restartGA returns a command to restart the GA with current tracks
func (m *model) restartGA() tea.Cmd {
	return func() tea.Msg {
		return gaRestartMsg{}
	}
}
