// ABOUTME: Progress tracking and update management for the genetic algorithm
// ABOUTME: Handles generation speed calculation and update channel communication

package main

import (
	"slices"
	"sync"
	"time"

	"playlist-sorter/playlist"
)

const (
	updateIntervalGenerations = 50 // Send updates every N generations (unless fitness improves)
)

// Tracker tracks progress update state
type Tracker struct {
	updateChan   chan<- GAUpdate
	sharedConfig *SharedConfig
	lastGenTime  time.Time
	lastGenCount int
	closeOnce    sync.Once
}

// SendUpdate sends a progress update to the channel if appropriate
func (pt *Tracker) SendUpdate(gen int, bestIndividual []playlist.Track, fitnessImproved bool) {
	// Guard: skip if not time to update or no channel
	if (!fitnessImproved && gen%updateIntervalGenerations != 0) || pt.updateChan == nil {
		return
	}

	// Calculate generation speed
	now := time.Now()
	elapsed := now.Sub(pt.lastGenTime).Seconds()
	genPerSec := 0.0
	if elapsed > 0 {
		genPerSec = float64(gen-pt.lastGenCount) / elapsed
	}

	// Get current config and send the all-time best individual with accurate breakdown
	config := pt.sharedConfig.Get()
	breakdown := calculateFitnessWithBreakdown(bestIndividual, config)

	select {
	case pt.updateChan <- GAUpdate{
		Generation:   gen,
		BestFitness:  breakdown.Total,
		BestPlaylist: slices.Clone(bestIndividual),
		GenPerSec:    genPerSec,
		Breakdown:    breakdown,
	}:
	default:
		// Don't block if channel is full
	}

	pt.lastGenTime = now
	pt.lastGenCount = gen
}

// Close ensures the update channel is closed exactly once
func (pt *Tracker) Close() {
	if pt.updateChan != nil {
		pt.closeOnce.Do(func() { close(pt.updateChan) })
	}
}
