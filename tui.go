// ABOUTME: Terminal UI for interactive genetic algorithm parameter tuning
// ABOUTME: Displays live playlist updates and allows real-time weight adjustment

package main

import (
	"context"
	"fmt"
	"time"

	"playlist-sorter/playlist"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// model holds the TUI state
type model struct {
	sharedConfig         *SharedConfig    // Shared config for GA thread-safe access
	localConfig          *GAConfig        // Local config that params point to (pointer so addresses stay valid)
	params               []Parameter      // Parameter list with pointers to localConfig fields
	selectedParam        int              // Currently selected parameter index
	bestPlaylist         []playlist.Track // Best playlist from GA
	originalTracks       []playlist.Track // Original tracks (for restart in Phase 5)
	bestFitness          float64          // Current best fitness
	previousBestFitness  float64          // Fitness at last improvement (for delta calculation)
	lastImprovementDelta float64          // Fitness improvement amount from last improvement
	breakdown            FitnessBreakdown // Fitness breakdown
	generation           int              // Current generation
	genPerSec            float64          // Generations per second
	lastImprovementTime  time.Time        // Time of last fitness improvement
	timeSinceImprovement time.Duration    // Duration since last improvement
	width                int
	height               int
	configPath           string             // Config file path
	playlistPath         string             // Playlist file path for auto-saving
	ctx                  context.Context    // Context for GA cancellation
	cancel               context.CancelFunc // Cancel function
	updateChan           chan GAUpdate      // Channel for GA updates
	quitting             bool
}

// Key bindings
type keyMap struct {
	Up    key.Binding
	Down  key.Binding
	Left  key.Binding
	Right key.Binding
	Reset key.Binding
	Quit  key.Binding
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "select param above"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "select param below"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "decrease value"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "increase value"),
	),
	Reset: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "reset to defaults"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
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
)

// initModel creates the initial model
func initModel(tracks []playlist.Track, configPath string, playlistPath string) model {
	config, _ := LoadConfig(configPath)

	// Create shared config for thread-safe GA access
	sharedConfig := &SharedConfig{
		config: config,
	}

	// Allocate localConfig on heap so pointers remain valid
	localConfig := &config

	// Create context for GA cancellation
	ctx, cancel := context.WithCancel(context.Background())

	m := model{
		sharedConfig:        sharedConfig,
		localConfig:         localConfig, // Store pointer to config
		selectedParam:       0,
		bestPlaylist:        tracks, // Start with original order
		originalTracks:      tracks,
		configPath:          configPath,
		playlistPath:        playlistPath,
		ctx:                 ctx,
		cancel:              cancel,
		updateChan:          make(chan GAUpdate, 10),
		lastImprovementTime: time.Now(), // Initialize to start time
	}

	// Build parameter list with pointers to localConfig fields
	// All fitness weights now use [0,1] range due to component normalization
	m.params = []Parameter{
		{"Harmonic Weight", &localConfig.HarmonicWeight, nil, 0, 1, 0.01, false},
		{"Energy Delta Weight", &localConfig.EnergyDeltaWeight, nil, 0, 1, 0.01, false},
		{"BPM Delta Weight", &localConfig.BPMDeltaWeight, nil, 0, 1, 0.01, false},
		{"Genre Weight", &localConfig.GenreWeight, nil, -1, 1, 0.01, false},
		{"Same Artist Penalty", &localConfig.SameArtistPenalty, nil, 0, 1, 0.01, false},
		{"Same Album Penalty", &localConfig.SameAlbumPenalty, nil, 0, 1, 0.01, false},
		{"Low Energy Bias Portion", &localConfig.LowEnergyBiasPortion, nil, 0, 1, 0.01, false},
		{"Low Energy Bias Weight", &localConfig.LowEnergyBiasWeight, nil, 0, 1, 0.01, false},
	}

	return m
}

// Init initializes the model
func (m model) Init() tea.Cmd {
	return tea.Batch(
		runGA(m.ctx, m.originalTracks, m.sharedConfig, m.updateChan),
		waitForGAUpdate(m.updateChan),
		tea.EnterAltScreen,
	)
}

// runGA starts the GA in a goroutine and returns a command
func runGA(ctx context.Context, tracks []playlist.Track, config *SharedConfig, updateChan chan<- GAUpdate) tea.Cmd {
	return func() tea.Msg {
		// Run GA (blocks until context cancelled or GA completes)
		progress := &progressTracker{
			updateChan:   updateChan,
			sharedConfig: config,
			lastGenTime:  time.Now(),
		}
		defer progress.close()
		geneticSort(ctx, tracks, config, progress)
		return nil
	}
}

// waitForGAUpdate waits for GA updates and returns them as messages
func waitForGAUpdate(updateChan <-chan GAUpdate) tea.Cmd {
	return func() tea.Msg {
		update, ok := <-updateChan
		if !ok {
			// Channel closed
			return nil
		}
		return update
	}
}

// Update handles messages and updates the model
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case GAUpdate:
		// Track fitness improvements for time-since-improvement display
		if msg.BestFitness < m.bestFitness || m.bestFitness == 0 {
			// Fitness improved! Calculate delta and update tracking
			oldFitness := m.bestFitness
			if oldFitness == 0 {
				// First update - use the initial fitness as baseline
				oldFitness = msg.BestFitness
			}
			m.lastImprovementDelta = oldFitness - msg.BestFitness
			m.previousBestFitness = oldFitness
			m.lastImprovementTime = time.Now()
		}

		// Update state with GA progress
		m.bestPlaylist = msg.BestPlaylist
		m.bestFitness = msg.BestFitness
		m.breakdown = msg.Breakdown
		m.generation = msg.Generation
		m.genPerSec = msg.GenPerSec
		m.timeSinceImprovement = time.Since(m.lastImprovementTime)

		// Auto-save the best playlist to disk
		if len(m.bestPlaylist) > 0 {
			_ = playlist.WritePlaylist(m.playlistPath, m.bestPlaylist)
		}

		// Queue next update
		return m, waitForGAUpdate(m.updateChan)

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			m.quitting = true
			// Cancel GA context
			m.cancel()
			// Save config on quit
			_ = SaveConfig(m.configPath, m.sharedConfig.Get())
			return m, tea.Quit

		case key.Matches(msg, keys.Up):
			if m.selectedParam > 0 {
				m.selectedParam--
			}

		case key.Matches(msg, keys.Down):
			if m.selectedParam < len(m.params)-1 {
				m.selectedParam++
			}

		case key.Matches(msg, keys.Left):
			m.decreaseParam()

		case key.Matches(msg, keys.Right):
			m.increaseParam()

		case key.Matches(msg, keys.Reset):
			m.resetToDefaults()
		}
	}

	return m, nil
}

// increaseParam increases the selected parameter value
func (m *model) increaseParam() {
	param := &m.params[m.selectedParam]
	if param.IsInt {
		newVal := *param.IntValue + int(param.Step)
		if float64(newVal) <= param.Max {
			*param.IntValue = newVal
		}
	} else {
		newVal := *param.Value + param.Step
		if newVal <= param.Max {
			*param.Value = newVal
		}
	}
	m.syncConfigToGA()
}

// decreaseParam decreases the selected parameter value
func (m *model) decreaseParam() {
	param := &m.params[m.selectedParam]
	if param.IsInt {
		newVal := *param.IntValue - int(param.Step)
		if float64(newVal) >= param.Min {
			*param.IntValue = newVal
		}
	} else {
		newVal := *param.Value - param.Step
		// Clamp to min if we're very close (handles floating point precision)
		if newVal < param.Min && newVal >= param.Min-0.0001 {
			newVal = param.Min
		}
		if newVal >= param.Min {
			*param.Value = newVal
		}
	}
	m.syncConfigToGA()
}

// syncConfigToGA syncs parameter values to the shared config
// Since all parameter pointers point to m.localConfig fields, we can simply
// copy the entire struct instead of iterating through parameters
func (m *model) syncConfigToGA() {
	// Parameters already modified m.localConfig directly via pointers
	// Just copy the entire struct to shared config (thread-safe)
	debugf("[TUI] Updating config - Genre Weight: %.2f", m.localConfig.GenreWeight)
	m.sharedConfig.Update(*m.localConfig)
	debugf("[TUI] Config update complete")
}

// resetToDefaults resets all parameters to their default values
func (m *model) resetToDefaults() {
	defaults := DefaultConfig()

	// Update parameter values to defaults
	*m.params[0].Value = defaults.HarmonicWeight
	*m.params[1].Value = defaults.EnergyDeltaWeight
	*m.params[2].Value = defaults.BPMDeltaWeight
	*m.params[3].Value = defaults.GenreWeight
	*m.params[4].Value = defaults.SameArtistPenalty
	*m.params[5].Value = defaults.SameAlbumPenalty
	*m.params[6].Value = defaults.LowEnergyBiasPortion
	*m.params[7].Value = defaults.LowEnergyBiasWeight

	// Sync to GA
	m.syncConfigToGA()
}

// View renders the TUI
func (m model) View() string {
	if m.quitting {
		return "Saving config and exiting...\n"
	}

	// Build the UI in two columns
	leftPanel := m.renderParameters()
	rightPanel := m.renderPlaylist()

	// Create styles for the two panels with fixed widths
	leftPanelStyle := lipgloss.NewStyle().
		Width(45).
		Padding(0, 1)

	rightPanelStyle := lipgloss.NewStyle().
		Width(m.width-47).
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

	s += titleStyle.Render("Algorithm parameters") + "\n\n"

	for i, param := range m.params {
		var value string
		if param.IsInt {
			value = fmt.Sprintf("%d", *param.IntValue)
		} else {
			value = fmt.Sprintf("%.2f", *param.Value)
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

// renderPlaylist renders the playlist preview
func (m model) renderPlaylist() string {
	var s string

	s += titleStyle.Render("Current best playlist") + "\n\n"

	// Header
	s += playlistHeaderStyle.Render(fmt.Sprintf("%-3s %-4s %-4s %-3s %-20s %-30s %-20s %-15s", "#", "Key", "BPM", "Eng", "Artist", "Title", "Album", "Genre")) + "\n"

	// Show top 25 tracks from best playlist
	maxTracks := 25
	if len(m.bestPlaylist) < maxTracks {
		maxTracks = len(m.bestPlaylist)
	}

	for i := 0; i < maxTracks; i++ {
		track := m.bestPlaylist[i]
		artist := truncate(track.Artist, 20)
		title := truncate(track.Title, 30)
		album := truncate(track.Album, 20)
		genre := truncate(track.Genre, 15)
		s += fmt.Sprintf("%-3d %-4s %-4.0f %-3d %-20s %-30s %-20s %-15s\n",
			i+1,
			track.Key,
			track.BPM,
			track.Energy,
			artist,
			title,
			album,
			genre,
		)
	}

	return s
}

// renderStatus renders the status bar
func (m model) renderStatus() string {
	// Format time since improvement in a readable way
	timeSince := m.timeSinceImprovement.Round(time.Second)

	// Show delta if we have improvement data
	deltaStr := ""
	if m.lastImprovementDelta != 0 {
		deltaStr = fmt.Sprintf(" | -%0.8f", m.lastImprovementDelta)
	}

	status := fmt.Sprintf("Gen: %d (%.1f gen/s) | Fitness: %.8f | %s ago%s",
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
	return helpStyle.Render(" ↑/↓: select param | ←/→: adjust value | d: reset defaults | q: quit & save")
}

// RunTUI starts the TUI mode
func RunTUI(playlistPath string) error {
	// Initialize debug logging
	if err := InitDebugLog("playlist-sorter-debug.log"); err != nil {
		return fmt.Errorf("failed to init debug log: %w", err)
	}

	// Load playlist (silent - no progress messages)
	tracks, err := playlist.LoadPlaylistWithMetadata(playlistPath, false)
	if err != nil {
		return fmt.Errorf("failed to load playlist: %w", err)
	}

	// Handle edge cases
	if len(tracks) == 0 {
		return fmt.Errorf("playlist is empty, nothing to optimize")
	}
	if len(tracks) == 1 {
		return fmt.Errorf("playlist has only one track, nothing to optimize")
	}

	// Assign Index values to tracks before any concurrent access
	for i := range tracks {
		tracks[i].Index = i
	}

	// Get config path
	configPath := GetConfigPath()

	// Create model
	m := initModel(tracks, configPath, playlistPath)

	// Run program
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	// Save the optimized playlist on exit
	if m, ok := finalModel.(model); ok && len(m.bestPlaylist) > 0 {
		if err := playlist.WritePlaylist(playlistPath, m.bestPlaylist); err != nil {
			return fmt.Errorf("failed to save playlist: %w", err)
		}
		fmt.Printf("\nSaved optimized playlist to: %s\n", playlistPath)
	}

	return nil
}
