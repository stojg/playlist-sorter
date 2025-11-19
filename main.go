package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
)

func main() {
	// Define flags
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to file")
	memprofile := flag.String("memprofile", "", "write memory profile to file")
	view := flag.Bool("view", false, "view playlist with live updates (no optimization)")
	visual := flag.Bool("visual", false, "run in visual/interactive mode with live parameter tuning")
	debug := flag.Bool("debug", false, "enable debug logging to playlist-sorter-debug.log")
	dryRun := flag.Bool("dry-run", false, "preview optimization without writing changes")
	output := flag.String("output", "", "write sorted playlist to this file (default: overwrite input)")
	flag.Parse()

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

	// Route to appropriate mode

	// View mode: Monitor playlist changes without optimization
	if *view {
		if err := RunViewMode(playlistPath); err != nil {
			log.Fatalf("View mode error: %v", err)
		}
		return
	}

	// TUI mode: Interactive parameter tuning
	if *visual {
		// TUI mode always enables debug logging internally
		if err := RunTUI(playlistPath); err != nil {
			log.Fatalf("TUI error: %v", err)
		}
		return
	}

	// CLI mode: Non-interactive optimization with profiling support

	// Start CPU profiling if requested (CLI mode only)
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

	// Run CLI mode
	if err := RunCLI(CLIOptions{
		PlaylistPath: playlistPath,
		DryRun:       *dryRun,
		OutputPath:   *output,
		DebugLog:     *debug,
	}); err != nil {
		log.Fatalf("CLI error: %v", err)
	}

	// Write memory profile if requested (CLI mode only)
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
