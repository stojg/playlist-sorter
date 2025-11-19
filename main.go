package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"syscall"
	"text/tabwriter"
	"time"

	"playlist-sorter/playlist"
)

func main() {
	// Define profiling flags
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to file")
	memprofile := flag.String("memprofile", "", "write memory profile to file")
	view := flag.Bool("view", false, "view playlist with live updates (no optimization)")
	visual := flag.Bool("visual", false, "run in visual/interactive mode with live parameter tuning")
	debug := flag.Bool("debug", false, "enable debug logging to playlist-sorter-debug.log")
	flag.Parse()

	// Enable debug logging if requested
	if *debug {
		if err := InitDebugLog("playlist-sorter-debug.log"); err != nil {
			log.Fatalf("Failed to initialize debug log: %v", err)
		}
		fmt.Println("Debug logging enabled: playlist-sorter-debug.log")
	}

	// Start CPU profiling if requested
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		defer func() {
			if err := f.Close(); err != nil {
				log.Printf("Warning: failed to close CPU profile: %v", err)
			}
		}()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	// Get playlist path from remaining args
	args := flag.Args()
	if len(args) != 1 {
		fmt.Println("Usage: playlist-sorter [flags] <playlist.m3u8>")
		fmt.Println("Example: playlist-sorter /Volumes/music/Music/low_energy_liquid_dnb.m3u8")
		fmt.Println("\nFlags:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	playlistPath := args[0]

	// Run in view mode if requested
	if *view {
		if err := RunViewMode(playlistPath); err != nil {
			log.Fatalf("View mode error: %v", err)
		}
		return
	}

	// Run in visual mode if requested
	if *visual {
		if err := RunTUI(playlistPath); err != nil {
			log.Fatalf("TUI error: %v", err)
		}
		// TUI handles everything, exit after it's done
		return
	}

	fmt.Printf("Reading playlist: %s\n", playlistPath)

	// Load playlist with metadata from beets (verbose mode for CLI)
	tracks, err := playlist.LoadPlaylistWithMetadata(playlistPath, true)
	if err != nil {
		log.Fatalf("Failed to load playlist: %v", err)
	}

	// Handle edge cases
	if len(tracks) == 0 {
		fmt.Println("Playlist is empty, nothing to optimize")
		return
	}

	if len(tracks) == 1 {
		fmt.Println("Playlist has only one track, nothing to optimize")
		return
	}

	// Assign Index values to tracks before any concurrent access
	for i := range tracks {
		tracks[i].Index = i
	}

	// Load config from file or use defaults
	config, _ := LoadConfig(GetConfigPath())

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

	// Wrap config in SharedConfig for consistency with TUI mode
	sharedConfig := &SharedConfig{
		config: config,
	}

	// Build edge fitness cache (required for fitness calculations)
	buildEdgeFitnessCache(tracks)

	// Calculate fitness bounds for context
	theoreticalMin := calculateTheoreticalMinimum(tracks, config)
	initialFitness := calculateFitness(tracks, config)

	fmt.Println("\nOptimizing playlist... (press Ctrl+C to stop early, or wait up to 1 hour)")
	fmt.Printf("Initial fitness: %.10f\n", initialFitness)
	fmt.Printf("Theoretical minimum: %.10f (not achievable, conflicting constraints)\n", theoreticalMin)
	fmt.Println()

	sortedTracks := cliGeneticSort(ctx, tracks, sharedConfig, playlistPath)

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

	// Write sorted playlist back
	fmt.Printf("\nWriting sorted playlist to: %s\n", playlistPath)
	if err := playlist.WritePlaylist(playlistPath, sortedTracks); err != nil {
		log.Fatalf("Failed to write playlist: %v", err)
	}
	fmt.Println("Done!")

	// Write memory profile if requested
	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Fatal("could not create memory profile: ", err)
		}
		defer func() {
			if err := f.Close(); err != nil {
				log.Printf("Warning: failed to close memory profile: %v", err)
			}
		}()
		runtime.GC() // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatal("could not write memory profile: ", err)
		}
	}
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
	go func() {
		result := geneticSort(ctx, tracks, config, updateChan)
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
