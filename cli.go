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

	"playlist-sorter/config"
	"playlist-sorter/playlist"
)

const (
	spinnerUpdateInterval     = 500 * time.Millisecond
	fitnessImprovementEpsilon = 1e-10
)

// isTTY checks if the given file is a terminal
func isTTY(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// RunCLI executes CLI mode optimization
func RunCLI(opts RunOptions) error {
	if opts.DebugLog {
		if err := SetupDebugLog("playlist-sorter-debug.log"); err != nil {
			return err
		}
	}

	data, err := InitializePlaylist(PlaylistOptions{
		Path:    opts.PlaylistPath,
		Verbose: true,
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-stop
		cancel()
	}()

	theoreticalMin := calculateTheoreticalMinimum(data.Tracks, data.Config, data.GACtx)
	initialFitness := calculateFitness(data.Tracks, data.Config, data.GACtx)

	fmt.Println("\nOptimizing playlist... (press Ctrl+C to stop early, or wait up to 15 minutes)")
	fmt.Printf("Initial fitness: %.10f\n", initialFitness)
	fmt.Printf("Theoretical minimum: %.10f (not achievable, conflicting constraints)\n", theoreticalMin)
	fmt.Println()

	sortedTracks := cliGeneticSort(ctx, data.Tracks, data.SharedConfig, data.GACtx, opts.PlaylistPath)

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

// cliGeneticSort wraps geneticSort with CLI-specific progress display
func cliGeneticSort(ctx context.Context, tracks []playlist.Track, sharedCfg *config.SharedConfig, gaCtx *GAContext, playlistPath string) []playlist.Track {
	startTime := time.Now()

	// Create update channel for tracking progress
	updateChan := make(chan GAUpdate, 10)

	// Track progress with pretty printing
	previousBestFitness := math.MaxFloat64
	minPrecision := 2 // Start with 2 decimals, increase monotonically as needed (max 10)

	// Detect if stdout is a TTY - no spinner needed in non-interactive contexts (cron, pipes, etc.)
	isTerminal := isTTY(os.Stdout)

	// Status line animation and ticker
	spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinnerIdx := 0

	var statusTicker *time.Ticker
	if isTerminal {
		statusTicker = time.NewTicker(spinnerUpdateInterval)
		defer statusTicker.Stop()
	}

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

	// Helper to print status line (overwrites itself in TTY, appends in non-TTY)
	printStatus := func(gen int) {
		if !isTerminal {
			// Non-TTY: skip spinner updates entirely to avoid log spam
			return
		}

		elapsed := time.Since(startTime)
		fmt.Printf("\r%s Gen %d %s     ", formatElapsed(elapsed), gen, spinnerFrames[spinnerIdx])
		spinnerIdx = (spinnerIdx + 1) % len(spinnerFrames)
	}

	// Start GA in goroutine
	var bestIndividual []playlist.Track

	done := make(chan []playlist.Track)

	defer close(updateChan)

	go func() {
		result := geneticSort(ctx, tracks, sharedCfg, updateChan, 0, gaCtx)
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
			fitnessImproved := hasFitnessImproved(update.BestFitness, previousBestFitness, fitnessImprovementEpsilon)

			if fitnessImproved {
				elapsed := time.Since(startTime)
				elapsedStr := formatElapsed(elapsed)

				if isTerminal {
					// Clear status line before printing progress (TTY only)
					fmt.Print("\r\033[K")
				}

				var fitnessStr string
				fitnessStr, minPrecision = FormatWithMonotonicPrecision(previousBestFitness, update.BestFitness, minPrecision)
				fmt.Printf("%s Gen %7d - fitness: %s\n", elapsedStr, currentGen, fitnessStr)
				previousBestFitness = update.BestFitness

				// Save playlist to disk for live monitoring with --view mode
				if err := playlist.WritePlaylist(playlistPath, update.BestPlaylist); err != nil {
					log.Printf("Warning: failed to write playlist: %v", err)
				}
			}

		case <-func() <-chan time.Time {
			if statusTicker != nil {
				return statusTicker.C
			}
			// Non-TTY: return never-firing channel
			return make(<-chan time.Time)
		}():
			printStatus(currentGen)

		case result := <-done:
			bestIndividual = result

			break loop
		}
	}

	// Clear status line at end (TTY only)
	if isTerminal {
		fmt.Print("\r\033[K")
	}

	fmt.Printf("\nCompleted %d generations in %v\n", currentGen, time.Since(startTime).Round(time.Millisecond))

	// Return best individual
	return bestIndividual
}
