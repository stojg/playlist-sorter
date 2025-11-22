// ABOUTME: Entry point for playlist-sorter application
// ABOUTME: Handles command-line parsing, profiling, and routing to CLI or TUI modes

// Package main provides the entry point for playlist-sorter, a genetic algorithm-based playlist optimizer.
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

	"playlist-sorter/config"
	"playlist-sorter/playlist"
	"playlist-sorter/tui"
)

func main() {
	os.Exit(run())
}

func run() int {
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to file")
	memprofile := flag.String("memprofile", "", "write memory profile to file")
	visual := flag.Bool("visual", false, "run in visual/interactive mode with live parameter tuning")
	debug := flag.Bool("debug", false, "enable debug logging to playlist-sorter-debug.log")
	dryRun := flag.Bool("dry-run", false, "preview optimization without writing changes")
	output := flag.String("output", "", "write sorted playlist to this file (default: overwrite input)")
	flag.Parse()

	args := flag.Args()
	if len(args) != 1 {
		fmt.Println("Usage: playlist-sorter [flags] <playlist.m3u8>")
		fmt.Println("Example: playlist-sorter /Volumes/music/Music/low_energy_liquid_dnb.m3u8")
		fmt.Println("\nFlags:")
		flag.PrintDefaults()

		return 1
	}

	playlistPath := args[0]

	if *cpuprofile != "" {
		stopCPUProfile := setupCPUProfile(*cpuprofile)
		defer stopCPUProfile()
	}

	if *memprofile != "" {
		defer writeMemoryProfile(*memprofile)
	}

	if *visual {
		if *debug {
			if err := SetupDebugLog("playlist-sorter-debug.log"); err != nil {
				log.Printf("Failed to setup debug log: %v", err)

				return 1
			}
		}

		opts := tui.Options{
			PlaylistPath: playlistPath,
			DryRun:       *dryRun,
			DebugLog:     *debug,
		}

		sharedCfg := &config.SharedConfig{}
		configPath := config.GetConfigPath()
		cfg, _ := config.LoadConfig(configPath)
		sharedCfg.Update(cfg)

		runGA := func(ctx context.Context, tracks []playlist.Track, updates chan<- tui.Update, epoch int) {
			runGAForTUI(ctx, tracks, sharedCfg, updates, epoch)
		}
		loadPlaylist := func(path string, requireMultiple bool) ([]playlist.Track, error) {
			allowSingle := !requireMultiple

			return LoadPlaylistForMode(PlaylistOptions{Path: path, Verbose: false}, allowSingle)
		}

		if err := tui.Run(opts, sharedCfg, runGA, loadPlaylist, playlist.WritePlaylist, debugf, configPath); err != nil {
			log.Printf("TUI error: %v", err)

			return 1
		}

		return 0
	}

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

// setupCPUProfile starts CPU profiling, returns cleanup function
func setupCPUProfile(filename string) func() {
	f, err := os.Create(filename)
	if err != nil {
		log.Fatalf("could not create CPU profile: %v", err)
	}

	if err := pprof.StartCPUProfile(f); err != nil {
		_ = f.Close()
		log.Fatalf("could not start CPU profile: %v", err)
	}

	return func() {
		pprof.StopCPUProfile()

		if err := f.Close(); err != nil {
			log.Printf("Warning: failed to close CPU profile: %v", err)
		}
	}
}

// writeMemoryProfile writes memory profile to file
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

	runtime.GC()

	if err := pprof.WriteHeapProfile(f); err != nil {
		log.Printf("could not write memory profile: %v", err)
	}
}

// runGAForTUI runs GA and converts updates to TUI format
func runGAForTUI(ctx context.Context, tracks []playlist.Track, sharedCfg *config.SharedConfig, updates chan<- tui.Update, epoch int) {
	// Buffer smooths GA update rate (updates sent every 50 gens or on improvement)
	gaUpdateChan := make(chan GAUpdate, 10)

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
				// Drain remaining updates
				for {
					select {
					case update, ok := <-gaUpdateChan:
						if !ok {
							return
						}

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
					return
				}

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
			}
		}
	}()

	gaCtx := buildEdgeFitnessCache(tracks)

	defer close(gaUpdateChan)

	geneticSort(ctx, tracks, sharedCfg, gaUpdateChan, epoch, gaCtx)
}
