package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/pprof"

	"playlist-sorter/config"
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

		// Create dependencies with adapters
		deps := tui.Dependencies{
			ConfigProvider: &configProviderAdapter{shared: &SharedConfig{}},
			GARunner:       &gaRunnerAdapter{},
			PlaylistLoader: &playlistLoaderAdapter{},
			PlaylistWriter: &playlistWriterAdapter{},
			Logger:         &loggerAdapter{},
			ConfigPath:     config.GetConfigPath(),
		}

		// Initialize the config provider with loaded config
		cfg, _ := config.LoadConfig(deps.ConfigPath)
		deps.ConfigProvider.(*configProviderAdapter).shared.Update(cfg)

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
