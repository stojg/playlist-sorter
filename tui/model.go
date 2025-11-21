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

// Layout constants for UI dimensions
const (
	paramPanelWidth = 45 // Left panel width for parameter controls
	panelPadding    = 2  // Horizontal spacing between panels

	// UI chrome heights (elements that reduce available viewport space)
	titleHeight     = 2 // Panel title bars
	headerHeight    = 1 // Column headers for playlist
	statusBarHeight = 1 // Bottom status bar
	breakdownHeight = 1 // Fitness breakdown display
	helpHeight      = 1 // Help text line
	spacingHeight   = 2 // Vertical spacing between elements
	totalUIChrome   = titleHeight + headerHeight + statusBarHeight + breakdownHeight + helpHeight + spacingHeight

	// Minimum viewport dimensions to ensure usability
	minViewportWidth  = 20
	minViewportHeight = 5
)

// Navigation and interaction constants
const (
	pageJumpSize          = 10              // Number of tracks to jump on PageUp/PageDown
	statusMessageDuration = 5 * time.Second // How long to show transient status messages
	maxUndoStackSize      = 50              // Maximum undo/redo history items
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
	// Dependencies (concrete types following Go philosophy)
	sharedConfig  *config.SharedConfig
	runGA         func(context.Context, []playlist.Track, chan<- Update, int)
	loadPlaylist  func(string, bool) ([]playlist.Track, error)
	writePlaylist func(string, []playlist.Track) error
	debugf        func(string, ...interface{})

	// Configuration
	localConfig *config.GAConfig // Local config that params point to (pointer so addresses stay valid)
	paramMgr    *ParamManager    // Parameter manager
	configPath  string           // Config file path

	// GA state
	bestPlaylist         []playlist.Track   // Best playlist from GA
	originalTracks       []playlist.Track   // Original tracks (for restart in Phase 5)
	bestFitness          float64            // Current best fitness
	previousBestFitness  float64            // Fitness at last improvement (for delta calculation)
	lastImprovementDelta float64            // Fitness improvement amount from last improvement
	breakdown            playlist.Breakdown // Fitness breakdown (shared type)
	generation           int                // Current generation
	genPerSec            float64            // Generations per second
	lastImprovementTime  time.Time          // Time of last fitness improvement
	timeSinceImprovement time.Duration      // Duration since last improvement

	// GA lifecycle
	// Framework exception: Context stored in struct because Bubble Tea's Init/Update/View
	// pattern doesn't allow passing context through function parameters. The framework owns
	// the model lifecycle, making context-in-struct the idiomatic pattern for cancellation.
	ctx        context.Context    //nolint:containedctx // See framework exception above
	cancel     context.CancelFunc // Cancel function for ctx
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
	cursorPos       int              // Current cursor position in track list
	viewport        viewport.Model   // Viewport for scrolling track list
	undoMgr         *UndoManager     // Undo/redo history manager
	editMode        bool             // True when user is manually editing (GA paused)
	displayedTracks []playlist.Track // Tracks shown to user (updated by GA or manual edits)
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
	tracks, err := deps.LoadPlaylist(opts.PlaylistPath, true)
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
			if err := deps.WritePlaylist(m.outputPath, m.bestPlaylist); err != nil {
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
	cfg := deps.SharedConfig.Get()

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
		// Injected dependencies (concrete types)
		sharedConfig:  deps.SharedConfig,
		runGA:         deps.RunGA,
		loadPlaylist:  deps.LoadPlaylist,
		writePlaylist: deps.WritePlaylist,
		debugf:        deps.Debugf,

		// Configuration
		localConfig: localConfig,
		configPath:  deps.ConfigPath,

		// GA state
		bestPlaylist:        tracks, // Start with original order
		originalTracks:      tracks,
		lastImprovementTime: time.Now(),

		// GA lifecycle
		ctx:    ctx,
		cancel: cancel,
		// Buffer of 10 balances responsiveness with smoothness:
		// - GA sends updates every 50 generations (~20/sec at 1000 gen/sec)
		// - Buffer allows ~0.5s of queued updates during brief TUI delays
		// - Not so large that we show stale data (e.g., gen 100 when at 5000)
		// - select-default in converter drops updates when full (prevents blocking GA)
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
		cursorPos:       0,
		displayedTracks: tracks,
		undoMgr:         NewUndoManager(maxUndoStackSize),
		editMode:        false,
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

// ========== Helpers ==========

// truncate shortens a string to maxLen, adding "..." if truncated
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	if maxLen <= 3 {
		return s[:maxLen]
	}

	return s[:maxLen-3] + "..."
}

// ========== Types and Dependencies ==========

// Dependencies holds the concrete dependencies needed by the TUI.
// Following Go philosophy: accept concrete types, not interfaces.
type Dependencies struct {
	SharedConfig  *config.SharedConfig
	RunGA         func(ctx context.Context, tracks []playlist.Track, updates chan<- Update, epoch int)
	LoadPlaylist  func(path string, requireMultiple bool) ([]playlist.Track, error)
	WritePlaylist func(path string, tracks []playlist.Track) error
	Debugf        func(format string, args ...interface{})
	ConfigPath    string
}

// Update represents a progress update from the GA
type Update struct {
	BestPlaylist []playlist.Track
	BestFitness  float64
	Breakdown    playlist.Breakdown // Using shared type from playlist package
	Generation   int
	GenPerSec    float64
	Epoch        int
}

// ========== Options ==========

// Options contains configuration for running the TUI
type Options struct {
	PlaylistPath string // Path to input playlist
	OutputPath   string // Path for saving (defaults to PlaylistPath)
	DryRun       bool   // If true, don't save changes to disk
	DebugLog     bool   // Enable debug logging to file
}

// ========== Parameter Manager ==========

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

// ResetToDefaults resets all parameters to their default values.
// Uses name-based lookup to avoid fragile array indexing.
func (pm *ParamManager) ResetToDefaults(defaults config.GAConfig) {
	for i := range pm.params {
		p := &pm.params[i]
		switch p.Name {
		case "Harmonic Weight":
			*p.Value = defaults.HarmonicWeight
		case "Energy Delta Weight":
			*p.Value = defaults.EnergyDeltaWeight
		case "BPM Delta Weight":
			*p.Value = defaults.BPMDeltaWeight
		case "Genre Weight":
			*p.Value = defaults.GenreWeight
		case "Same Artist Penalty":
			*p.Value = defaults.SameArtistPenalty
		case "Same Album Penalty":
			*p.Value = defaults.SameAlbumPenalty
		case "Low Energy Bias Portion":
			*p.Value = defaults.LowEnergyBiasPortion
		case "Low Energy Bias Weight":
			*p.Value = defaults.LowEnergyBiasWeight
		}
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

// ========== Undo Manager ==========

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

// ========== Viewport Manager ==========

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

// ========== Bubble Tea Lifecycle ==========
// Init initializes the model
func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.startGA(m.ctx, m.originalTracks, m.gaEpoch),
		waitForUpdate(m.updateChan),
		tea.EnterAltScreen,
	)
}

// ========== Helper Methods ==========

// startGA starts the GA in a goroutine and returns a command
func (m *model) startGA(ctx context.Context, tracks []playlist.Track, epoch int) tea.Cmd {
	return func() tea.Msg {
		defer func() {
			if r := recover(); r != nil {
				m.debugf("[PANIC] startGA panic: %v", r)
				m.debugf("[PANIC] Stack trace: %s", string(debug.Stack()))
				panic(r) // Re-panic after logging
			}
		}()

		// Run GA via injected function (blocks until context cancelled or GA completes)
		m.runGA(ctx, tracks, m.updateChan, epoch)
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
		m.debugf("[TUI] Parameter changed - %s: %.2f (Harmonic: %.2f, Energy: %.2f, BPM: %.2f)",
			selected.Name,
			value,
			m.localConfig.HarmonicWeight,
			m.localConfig.EnergyDeltaWeight,
			m.localConfig.BPMDeltaWeight)
	}
	m.sharedConfig.Update(*m.localConfig)

	// Increment epoch immediately to invalidate any pending GA updates with old weights
	m.gaEpoch++

	m.debugf("[TUI] Config synced - restarting GA with epoch %d for new weights", m.gaEpoch)

	// Restart GA with new weights (same tracks, new epoch)
	return m.restartGA()
}

// setStatusMsg sets a transient status message with current timestamp
func (m *model) setStatusMsg(msg string) {
	m.statusMsg = msg
	m.statusMsgAge = time.Now()
}

// ensureCursorVisible adjusts viewport offset to keep cursor visible with middle-of-screen scrolling
// Implements vim/less style scrolling using ViewportManager
func (m *model) ensureCursorVisible() {
	vm := NewViewportManager(m.viewport.Height, m.cursorPos, len(m.displayedTracks))
	offset := vm.CalculateOffset()
	m.viewport.SetYOffset(offset)
}

// pushUndo saves current state to undo stack using UndoManager
func (m *model) pushUndo() {
	state := PlaylistState{
		Tracks:    m.displayedTracks,
		CursorPos: m.cursorPos,
	}
	m.undoMgr.Push(state)
}

// deleteTrack removes the track at cursor position and restarts GA
func (m *model) deleteTrack() tea.Cmd {
	if len(m.displayedTracks) == 0 {
		return nil
	}

	// Save current state to undo stack
	m.pushUndo()

	// Remove track at cursor
	m.displayedTracks = append(m.displayedTracks[:m.cursorPos], m.displayedTracks[m.cursorPos+1:]...)

	// Set edit mode
	m.editMode = true

	// Increment epoch immediately to invalidate any pending GA updates
	m.gaEpoch++

	// Adjust cursor if needed
	if m.cursorPos >= len(m.displayedTracks) && len(m.displayedTracks) > 0 {
		m.cursorPos = len(m.displayedTracks) - 1
	}

	// Update status message
	m.setStatusMsg(fmt.Sprintf("Deleted track (Undo: %d, Redo: %d)", m.undoMgr.UndoSize(), m.undoMgr.RedoSize()))

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
		Tracks:    m.displayedTracks,
		CursorPos: m.cursorPos,
	}

	state, ok := m.undoMgr.Undo(currentState)
	if !ok {
		m.setStatusMsg("Nothing to undo")
		return nil
	}

	// Restore state
	m.displayedTracks = state.Tracks
	m.cursorPos = state.CursorPos
	m.ensureCursorVisible()

	// Increment epoch immediately to invalidate any pending GA updates
	m.gaEpoch++

	// Update status message
	m.setStatusMsg(fmt.Sprintf("Undo (Undo: %d, Redo: %d)", m.undoMgr.UndoSize(), m.undoMgr.RedoSize()))

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
		Tracks:    m.displayedTracks,
		CursorPos: m.cursorPos,
	}

	state, ok := m.undoMgr.Redo(currentState)
	if !ok {
		m.setStatusMsg("Nothing to redo")
		return nil
	}

	// Restore state
	m.displayedTracks = state.Tracks
	m.cursorPos = state.CursorPos
	m.ensureCursorVisible()

	// Increment epoch immediately to invalidate any pending GA updates
	m.gaEpoch++

	// Update status message
	m.setStatusMsg(fmt.Sprintf("Redo (Undo: %d, Redo: %d)", m.undoMgr.UndoSize(), m.undoMgr.RedoSize()))

	// Update viewport
	m.updateViewportContent()

	// Auto-save the restored playlist
	m.autoSave()

	// Restart GA with restored tracks
	return m.restartGA()
}

// autoSave writes current tracks to disk
func (m *model) autoSave() {
	if m.dryRun {
		return
	}

	if err := m.writePlaylist(m.outputPath, m.displayedTracks); err != nil {
		m.debugf("[TUI] Auto-save failed: %v", err)
	} else {
		m.debugf("[TUI] Auto-saved %d tracks to %s", len(m.displayedTracks), m.outputPath)
	}
}

// restartGA returns a command to restart the GA with current tracks
func (m *model) restartGA() tea.Cmd {
	return func() tea.Msg {
		return gaRestartMsg{}
	}
}
