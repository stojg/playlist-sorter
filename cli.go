// ABOUTME: CLI mode implementation for non-interactive playlist optimization
// ABOUTME: Handles progress display, result output, and signal handling for command-line usage

package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	"playlist-sorter/playlist"
)

// CLIOptions contains options for CLI mode
type CLIOptions struct {
	PlaylistPath string
	DryRun       bool
	OutputPath   string
	DebugLog     bool
}

// RunCLI executes CLI mode optimization
func RunCLI(opts CLIOptions) error {
	// Setup debug logging if requested
	if opts.DebugLog {
		if err := SetupDebugLog("playlist-sorter-debug.log"); err != nil {
			return err
		}
	}

	// Initialize playlist with full setup (config, cache, etc.)
	data, err := InitializePlaylist(PlaylistOptions{
		Path:    opts.PlaylistPath,
		Verbose: true,
	})
	if err != nil {
		return err
	}

	// Set up context with cancellation for Ctrl+C
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for Ctrl+C
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		cancel()
	}()

	// Calculate fitness bounds for context
	theoreticalMin := calculateTheoreticalMinimum(data.Tracks, data.Config)
	initialFitness := calculateFitness(data.Tracks, data.Config)

	fmt.Println("\nOptimizing playlist... (press Ctrl+C to stop early, or wait up to 1 hour)")
	fmt.Printf("Initial fitness: %.10f\n", initialFitness)
	fmt.Printf("Theoretical minimum: %.10f (not achievable, conflicting constraints)\n", theoreticalMin)
	fmt.Println()

	sortedTracks := cliGeneticSort(ctx, data.Tracks, data.SharedConfig, opts.PlaylistPath)

	// Show sorted playlist with tabwriter
	fmt.Println("\nSorted playlist:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "#\tKey\tBPM\tEng\tArtist\tTitle\tAlbum\tGenre"); err != nil {
		log.Printf("Warning: failed to write header: %v", err)
	}
	if _, err := fmt.Fprintln(w, "---\t---\t---\t---\t------\t-----\t-----\t-----"); err != nil {
		log.Printf("Warning: failed to write separator: %v", err)
	}

	for i, track := range sortedTracks {
		if _, err := fmt.Fprintf(w, "%d\t%s\t%.0f\t%d\t%s\t%s\t%s\t%s\n",
			i+1,
			track.Key,
			track.BPM,
			track.Energy,
			truncate(track.Artist, 20),
			truncate(track.Title, 30),
			truncate(track.Album, 20),
			truncate(track.Genre, 15),
		); err != nil {
			log.Printf("Warning: failed to write track %d: %v", i+1, err)
		}
	}
	if err := w.Flush(); err != nil {
		log.Printf("Warning: failed to flush output: %v", err)
	}

	// Write sorted playlist (or just preview with --dry-run)
	if opts.DryRun {
		fmt.Println("\n--dry-run mode: playlist not modified")
	} else {
		outputPath := opts.PlaylistPath
		if opts.OutputPath != "" {
			outputPath = opts.OutputPath
		}
		fmt.Printf("\nWriting sorted playlist to: %s\n", outputPath)
		if err := playlist.WritePlaylist(outputPath, sortedTracks); err != nil {
			return fmt.Errorf("failed to write playlist: %w", err)
		}
		fmt.Println("Done!")
	}

	return nil
}

// truncate truncates a string to maxLen characters, adding "..." if needed
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// cliGeneticSort wraps geneticSort with CLI-specific progress display
func cliGeneticSort(ctx context.Context, tracks []playlist.Track, config *SharedConfig, playlistPath string) []playlist.Track {
	startTime := time.Now()

	// Create update channel for tracking progress
	updateChan := make(chan GAUpdate, 10)

	// Track progress with pretty printing
	var previousBestFitness float64 = math.MaxFloat64

	// Status line animation and ticker
	spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinnerIdx := 0
	statusTicker := time.NewTicker(500 * time.Millisecond)
	defer statusTicker.Stop()

	// Helper to format elapsed time (right-padded to 6 chars for max "59m59s")
	formatElapsed := func(d time.Duration) string {
		var s string
		if d >= time.Minute {
			s = fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
		} else {
			s = fmt.Sprintf("%ds", int(d.Seconds()))
		}
		return fmt.Sprintf("%6s", s) // Right-align to 6 characters
	}

	// Helper to print status line (overwrites itself)
	printStatus := func(gen int) {
		elapsed := time.Since(startTime)
		fmt.Printf("\r%s Gen %d %s     ", formatElapsed(elapsed), gen, spinnerFrames[spinnerIdx])
		spinnerIdx = (spinnerIdx + 1) % len(spinnerFrames)
	}

	// Start GA in goroutine
	var bestIndividual []playlist.Track
	done := make(chan []playlist.Track)
	progress := &Tracker{
		updateChan:   updateChan,
		sharedConfig: config,
		lastGenTime:  startTime,
	}
	defer progress.Close()
	go func() {
		result := geneticSort(ctx, tracks, config, progress)
		done <- result
	}()

	// Monitor updates and print progress
	var currentGen int
loop:
	for {
		select {
		case update, ok := <-updateChan:
			if !ok {
				// Channel closed, wait for result
				bestIndividual = <-done
				break loop
			}
			currentGen = update.Generation

			// Print progress when fitness improves
			fitnessImproved := update.BestFitness < previousBestFitness-1e-10 // Use epsilon to avoid float precision issues

			if fitnessImproved {
				// Clear status line before printing progress
				fmt.Print("\r\033[K")
				elapsed := time.Since(startTime)
				elapsedStr := formatElapsed(elapsed)
				fmt.Printf("%s Gen %d - fitness: %.10f\n", elapsedStr, currentGen, update.BestFitness)
				previousBestFitness = update.BestFitness

				// Save playlist to disk for live monitoring with --view mode
				if err := playlist.WritePlaylist(playlistPath, update.BestPlaylist); err != nil {
					log.Printf("Warning: failed to write playlist: %v", err)
				}
			}

		case <-statusTicker.C:
			printStatus(currentGen)

		case result := <-done:
			bestIndividual = result
			break loop
		}
	}

	// Clear status line at end
	fmt.Print("\r\033[K")

	fmt.Printf("\nCompleted %d generations in %v\n", currentGen, time.Since(startTime).Round(time.Millisecond))

	// Return best individual
	return bestIndividual
}
