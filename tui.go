// ABOUTME: Terminal UI for interactive genetic algorithm parameter tuning
// ABOUTME: Displays live playlist updates and allows real-time weight adjustment

package main

import (
	"context"
	"fmt"

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
	sharedConfig   *SharedConfig    // Shared config for GA thread-safe access
	params         []Parameter      // Parameter list with pointers to local config
	selectedParam  int              // Currently selected parameter index
	bestPlaylist   []playlist.Track // Best playlist from GA
	originalTracks []playlist.Track // Original tracks (for restart in Phase 5)
	bestFitness    float64          // Current best fitness
	breakdown      FitnessBreakdown // Fitness breakdown
	generation     int              // Current generation
	genPerSec      float64          // Generations per second
	width          int
	height         int
	configPath     string             // Config file path
	ctx            context.Context    // Context for GA cancellation
	cancel         context.CancelFunc // Cancel function
	updateChan     chan GAUpdate      // Channel for GA updates
	quitting       bool
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
func initModel(tracks []playlist.Track, configPath string) model {
	config, _ := LoadConfig(configPath)

	// Create shared config for thread-safe GA access
	sharedConfig := &SharedConfig{
		config: config,
	}

	// Create context for GA cancellation
	ctx, cancel := context.WithCancel(context.Background())

	// Create local config copy for parameter binding
	localConfig := config

	m := model{
		sharedConfig:   sharedConfig,
		selectedParam:  0,
		bestPlaylist:   tracks, // Start with original order
		originalTracks: tracks,
		configPath:     configPath,
		ctx:            ctx,
		cancel:         cancel,
		updateChan:     make(chan GAUpdate, 10),
	}

	// Build parameter list with pointers to local config fields
	// All fitness weights now use [0,1] range due to component normalization
	m.params = []Parameter{
		{"Harmonic Weight", &localConfig.HarmonicWeight, nil, 0, 1, 0.05, false},
		{"Energy Delta Weight", &localConfig.EnergyDeltaWeight, nil, 0, 1, 0.05, false},
		{"BPM Delta Weight", &localConfig.BPMDeltaWeight, nil, 0, 1, 0.05, false},
		{"Same Artist Penalty", &localConfig.SameArtistPenalty, nil, 0, 1, 0.05, false},
		{"Same Album Penalty", &localConfig.SameAlbumPenalty, nil, 0, 1, 0.05, false},
		{"Low Energy Bias Portion", &localConfig.LowEnergyBiasPortion, nil, 0, 1, 0.01, false},
		{"Low Energy Bias Weight", &localConfig.LowEnergyBiasWeight, nil, 0, 1, 0.05, false},
		{"Max Mutation Rate", &localConfig.MaxMutationRate, nil, 0, 1, 0.01, false},
		{"Min Mutation Rate", &localConfig.MinMutationRate, nil, 0, 1, 0.01, false},
		{"Mutation Decay Gen", &localConfig.MutationDecayGen, nil, 10, 1000, 1, false},
		{"Min Swap Mutations", nil, &localConfig.MinSwapMutations, 1, 20, 1, true},
		{"Max Swap Mutations", nil, &localConfig.MaxSwapMutations, 1, 50, 1, true},
		{"Population Size", nil, &localConfig.PopulationSize, 10, 1000, 10, true},
		{"Immigration Rate", &localConfig.ImmigrationRate, nil, 0, 1, 0.01, false},
		{"Elite Percentage", &localConfig.ElitePercentage, nil, 0, 1, 0.01, false},
		{"Tournament Size", nil, &localConfig.TournamentSize, 2, 20, 1, true},
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
		geneticSort(ctx, tracks, config, updateChan)
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
		// Update state with GA progress
		m.bestPlaylist = msg.BestPlaylist
		m.bestFitness = msg.BestFitness
		m.breakdown = msg.Breakdown
		m.generation = msg.Generation
		m.genPerSec = msg.GenPerSec

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
func (m *model) syncConfigToGA() {
	// Build config from parameter values
	config := GAConfig{}

	// Read from parameter pointers
	config.HarmonicWeight = *m.params[0].Value
	config.SameArtistPenalty = *m.params[1].Value
	config.SameAlbumPenalty = *m.params[2].Value
	config.EnergyDeltaWeight = *m.params[3].Value
	config.BPMDeltaWeight = *m.params[4].Value
	config.LowEnergyBiasPortion = *m.params[5].Value
	config.LowEnergyBiasWeight = *m.params[6].Value
	config.MaxMutationRate = *m.params[7].Value
	config.MinMutationRate = *m.params[8].Value
	config.MutationDecayGen = *m.params[9].Value
	config.MinSwapMutations = *m.params[10].IntValue
	config.MaxSwapMutations = *m.params[11].IntValue
	config.PopulationSize = *m.params[12].IntValue
	config.ImmigrationRate = *m.params[13].Value
	config.ElitePercentage = *m.params[14].Value
	config.TournamentSize = *m.params[15].IntValue

	// Update shared config (thread-safe)
	m.sharedConfig.Update(config)
}

// resetToDefaults resets all parameters to their default values
func (m *model) resetToDefaults() {
	defaults := DefaultConfig()

	// Update parameter values to defaults
	*m.params[0].Value = defaults.HarmonicWeight
	*m.params[1].Value = defaults.SameArtistPenalty
	*m.params[2].Value = defaults.SameAlbumPenalty
	*m.params[3].Value = defaults.EnergyDeltaWeight
	*m.params[4].Value = defaults.BPMDeltaWeight
	*m.params[5].Value = defaults.LowEnergyBiasPortion
	*m.params[6].Value = defaults.LowEnergyBiasWeight
	*m.params[7].Value = defaults.MaxMutationRate
	*m.params[8].Value = defaults.MinMutationRate
	*m.params[9].Value = defaults.MutationDecayGen
	*m.params[10].IntValue = defaults.MinSwapMutations
	*m.params[11].IntValue = defaults.MaxSwapMutations
	*m.params[12].IntValue = defaults.PopulationSize
	*m.params[13].Value = defaults.ImmigrationRate
	*m.params[14].Value = defaults.ElitePercentage
	*m.params[15].IntValue = defaults.TournamentSize

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

	s += titleStyle.Render("Algorithm Parameters") + "\n\n"

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

	s += titleStyle.Render("Best Playlist Preview") + "\n\n"

	// Header
	s += playlistHeaderStyle.Render(fmt.Sprintf("%-3s %-4s %-4s %-3s %-20s %-20s", "#", "Key", "BPM", "Eng", "Artist", "Title")) + "\n"

	// Show top 15 tracks from best playlist
	maxTracks := 15
	if len(m.bestPlaylist) < maxTracks {
		maxTracks = len(m.bestPlaylist)
	}

	for i := 0; i < maxTracks; i++ {
		track := m.bestPlaylist[i]
		artist := truncate(track.Artist, 20)
		title := truncate(track.Title, 20)
		s += fmt.Sprintf("%-3d %-4s %-4.0f %-3d %-20s %-20s\n",
			i+1,
			track.Key,
			track.BPM,
			track.Energy,
			artist,
			title,
		)
	}

	return s
}

// renderStatus renders the status bar
func (m model) renderStatus() string {
	status := fmt.Sprintf("Gen: %d | Fitness: %.2f | Speed: %.1f gen/s",
		m.generation,
		m.bestFitness,
		m.genPerSec,
	)
	return statusStyle.Width(m.width).Render(status)
}

// renderBreakdown renders the fitness breakdown showing individual components
func (m model) renderBreakdown() string {
	if m.breakdown.Total == 0 {
		// No breakdown available yet
		return ""
	}
	breakdown := fmt.Sprintf("Breakdown: Harmonic: %.3f | Artist: %.3f | Album: %.3f | Energy: %.3f | BPM: %.3f | Bias: %.3f",
		m.breakdown.Harmonic,
		m.breakdown.SameArtist,
		m.breakdown.SameAlbum,
		m.breakdown.EnergyDelta,
		m.breakdown.BPMDelta,
		m.breakdown.PositionBias,
	)
	return helpStyle.Render(breakdown)
}

// renderSparkline renders a simple ASCII sparkline of fitness history (Phase 6)
func (m model) renderSparkline() string {
	// TODO: Implement in Phase 6
	return ""
}

// renderHelp renders the help text
func (m model) renderHelp() string {
	return helpStyle.Render("↑/↓: select param | ←/→: adjust value | d: reset defaults | q: quit & save")
}

// RunTUI starts the TUI mode
func RunTUI(playlistPath string) error {
	// Load playlist
	tracks, err := playlist.LoadPlaylistWithMetadata(playlistPath)
	if err != nil {
		return fmt.Errorf("failed to load playlist: %w", err)
	}

	// Assign Index values to tracks before any concurrent access
	for i := range tracks {
		tracks[i].Index = i
	}

	// Get config path
	configPath := GetConfigPath()

	// Create model
	m := initModel(tracks, configPath)

	// Run program
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}
