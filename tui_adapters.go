// ABOUTME: Adapter implementations for TUI interfaces
// ABOUTME: Bridges main package implementations to TUI interface contracts

package main

import (
	"context"
	"runtime/debug"
	"time"

	"playlist-sorter/config"
	"playlist-sorter/playlist"
	"playlist-sorter/tui"
)

// configProviderAdapter adapts SharedConfig to tui.ConfigProvider interface
type configProviderAdapter struct {
	shared *SharedConfig
}

func (c *configProviderAdapter) Get() config.GAConfig {
	return c.shared.Get()
}

func (c *configProviderAdapter) Update(cfg config.GAConfig) {
	c.shared.Update(cfg)
}

// gaRunnerAdapter adapts the geneticSort function to tui.GARunner interface
type gaRunnerAdapter struct{}

func (g *gaRunnerAdapter) Run(ctx context.Context, tracks []playlist.Track, cfg tui.ConfigProvider, updates chan<- tui.Update, epoch int) {
	// Convert ConfigProvider back to SharedConfig for geneticSort
	// (geneticSort expects *SharedConfig, not the interface)
	sharedCfg := &SharedConfig{}
	sharedCfg.Update(cfg.Get())

	// Create a converter channel to transform GAUpdate -> tui.Update
	gaUpdateChan := make(chan GAUpdate, 10)

	// Start converter goroutine
	go func() {
		defer func() {
			if r := recover(); r != nil {
				debugf("[PANIC] Converter goroutine panic: %v\n%s", r, string(debug.Stack()))
				panic(r) // Re-panic after logging
			}
		}()

		for update := range gaUpdateChan {
			tuiUpdate := tui.Update{
				BestPlaylist: update.BestPlaylist,
				BestFitness:  update.BestFitness,
				Breakdown: tui.Breakdown{
					Total:        update.Breakdown.Total,
					Harmonic:     update.Breakdown.Harmonic,
					EnergyDelta:  update.Breakdown.EnergyDelta,
					BPMDelta:     update.Breakdown.BPMDelta,
					GenreChange:  update.Breakdown.GenreChange,
					SameArtist:   update.Breakdown.SameArtist,
					SameAlbum:    update.Breakdown.SameAlbum,
					PositionBias: update.Breakdown.PositionBias,
				},
				Generation: update.Generation,
				GenPerSec:  update.GenPerSec,
				Epoch:      update.Epoch,
			}

			select {
			case updates <- tuiUpdate:
			default:
				// Channel full, skip update
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
	defer func() {
		// tracker.Close() already closes gaUpdateChan with its own sync.Once
		tracker.Close()
	}()

	geneticSort(ctx, tracks, sharedCfg, tracker)
}

// playlistLoaderAdapter adapts LoadPlaylistForMode to tui.PlaylistLoader interface
type playlistLoaderAdapter struct{}

func (p *playlistLoaderAdapter) Load(path string, requireMultiple bool) ([]playlist.Track, error) {
	validation := AllowSingleTrack
	if requireMultiple {
		validation = RequireMultipleTracks
	}

	return LoadPlaylistForMode(PlaylistOptions{
		Path:    path,
		Verbose: false,
	}, validation)
}

// playlistWriterAdapter adapts playlist.WritePlaylist to tui.PlaylistWriter interface
type playlistWriterAdapter struct{}

func (p *playlistWriterAdapter) Write(path string, tracks []playlist.Track) error {
	return playlist.WritePlaylist(path, tracks)
}

// loggerAdapter adapts debugf to tui.Logger interface
type loggerAdapter struct{}

func (l *loggerAdapter) Debugf(format string, args ...interface{}) {
	debugf(format, args...)
}
