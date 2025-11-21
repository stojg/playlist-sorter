// ABOUTME: Entry point for playlist-sorter application
// ABOUTME: Handles command-line parsing, profiling, and routing to CLI or TUI modes

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"time"

	"playlist-sorter/config"
	"playlist-sorter/playlist"
	"playlist-sorter/tui"
)

func main() {
	os.Exit(run())
}

func run() int {
	// Parse command-line flags
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to file")
	memprofile := flag.String("memprofile", "", "write memory profile to file")
	visual := flag.Bool("visual", false, "run in visual/interactive mode with live parameter tuning")
	debug := flag.Bool("debug", false, "enable debug logging to playlist-sorter-debug.log")
	dryRun := flag.Bool("dry-run", false, "preview optimization without writing changes")
	output := flag.String("output", "", "write sorted playlist to this file (default: overwrite input)")
	flag.Parse()

	// Validate arguments
	args := flag.Args()
	if len(args) != 1 {
		fmt.Println("Usage: playlist-sorter [flags] <playlist.m3u8>")
		fmt.Println("Example: playlist-sorter /Volumes/music/Music/low_energy_liquid_dnb.m3u8")
		fmt.Println("\nFlags:")
		flag.PrintDefaults()

		return 1
	}

	playlistPath := args[0]

	// Setup profiling (works for all modes)
	if *cpuprofile != "" {
		stopCPUProfile := setupCPUProfile(*cpuprofile)
		defer stopCPUProfile()
	}

	if *memprofile != "" {
		defer writeMemoryProfile(*memprofile)
	}

	// Route to appropriate mode
	if *visual {
		// Setup debug logging if requested
		if *debug {
			if err := SetupDebugLog("playlist-sorter-debug.log"); err != nil {
				log.Printf("Failed to setup debug log: %v", err)
				return 1
			}
		}

		// Create TUI options
		opts := tui.Options{
			PlaylistPath: playlistPath,
			DryRun:       *dryRun,
			OutputPath:   *output,
			DebugLog:     *debug,
		}

		// Create shared config and initialize with loaded config
		sharedCfg := &SharedConfig{}
		configPath := config.GetConfigPath()
		cfg, _ := config.LoadConfig(configPath)
		sharedCfg.Update(cfg)

		// Create dependencies with concrete implementations (Go philosophy: no adapters!)
		deps := tui.Dependencies{
			SharedConfig: sharedCfg,
			RunGA: func(ctx context.Context, tracks []playlist.Track, updates chan<- tui.Update, epoch int) {
				runGAForTUI(ctx, tracks, sharedCfg, updates, epoch)
			},
			LoadPlaylist: func(path string, requireMultiple bool) ([]playlist.Track, error) {
				allowSingle := !requireMultiple
				return LoadPlaylistForMode(PlaylistOptions{Path: path, Verbose: false}, allowSingle)
			},
			WritePlaylist: playlist.WritePlaylist,
			Debugf:        debugf,
			ConfigPath:    configPath,
		}

		if err := tui.Run(opts, deps); err != nil {
			log.Printf("TUI error: %v", err)

			return 1
		}

		return 0
	}

	// Default to CLI mode
	if err := RunCLI(RunOptions{
		PlaylistPath: playlistPath,
		DryRun:       *dryRun,
		OutputPath:   *output,
		DebugLog:     *debug,
	}); err != nil {
		log.Printf("CLI error: %v", err)

		return 1
	}

	return 0
}

// setupCPUProfile starts CPU profiling and returns a cleanup function
func setupCPUProfile(filename string) func() {
	f, err := os.Create(filename)
	if err != nil {
		log.Fatalf("could not create CPU profile: %v", err)
	}

	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close()
		log.Fatalf("could not start CPU profile: %v", err)
	}

	return func() {
		pprof.StopCPUProfile()

		if err := f.Close(); err != nil {
			log.Printf("Warning: failed to close CPU profile: %v", err)
		}
	}
}

// writeMemoryProfile writes a memory profile to the specified file
func writeMemoryProfile(filename string) {
	f, err := os.Create(filename)
	if err != nil {
		log.Printf("could not create memory profile: %v", err)

		return
	}

	defer func() {
		if err := f.Close(); err != nil {
			log.Printf("Warning: failed to close memory profile: %v", err)
		}
	}()

	runtime.GC() // Get up-to-date statistics

	if err := pprof.WriteHeapProfile(f); err != nil {
		log.Printf("could not write memory profile: %v", err)
	}
}

// runGAForTUI runs the genetic algorithm and converts updates to TUI format.
// This replaces the old gaRunnerAdapter without interface ceremony.
func runGAForTUI(ctx context.Context, tracks []playlist.Track, sharedCfg *SharedConfig, updates chan<- tui.Update, epoch int) {
	// Create converter channel for GA updates
	// Buffer of 10 provides smoothing between GA update rate and converter processing:
	// - GA sends updates every 50 generations OR on fitness improvement
	// - Buffer prevents GA blocking during brief converter delays
	// - select-default in progress.SendUpdate drops updates when full
	gaUpdateChan := make(chan GAUpdate, 10)

	// Start converter goroutine with explicit context handling
	go func() {
		defer func() {
			if r := recover(); r != nil {
				debugf("[PANIC] Converter goroutine panic: %v\n%s", r, string(debug.Stack()))
				panic(r)
			}
		}()

		for {
			select {
			case <-ctx.Done():
				// Context cancelled (TUI exited) - drain remaining updates and exit
				for {
					select {
					case update, ok := <-gaUpdateChan:
						if !ok {
							return
						}
						// Process final update
						tuiUpdate := tui.Update{
							BestPlaylist: update.BestPlaylist,
							BestFitness:  update.BestFitness,
							Breakdown:    update.Breakdown,
							Generation:   update.Generation,
							GenPerSec:    update.GenPerSec,
							Epoch:        update.Epoch,
						}
						select {
						case updates <- tuiUpdate:
						default:
						}
					default:
						return
					}
				}
			case update, ok := <-gaUpdateChan:
				if !ok {
					return // Channel closed, exit cleanly
				}

				tuiUpdate := tui.Update{
					BestPlaylist: update.BestPlaylist,
					BestFitness:  update.BestFitness,
					Breakdown:    update.Breakdown, // Same type now - no field copying needed!
					Generation:   update.Generation,
					GenPerSec:    update.GenPerSec,
					Epoch:        update.Epoch,
				}

				select {
				case updates <- tuiUpdate:
				default:
					// Channel full, skip update
				}
			}
		}
	}()

	// Create tracker with the GA update channel
	tracker := &Tracker{
		updateChan:   gaUpdateChan,
		sharedConfig: sharedCfg,
		epoch:        epoch,
		lastGenTime:  time.Now(),
	}
	defer tracker.Close()

	geneticSort(ctx, tracks, sharedCfg, tracker)
}
