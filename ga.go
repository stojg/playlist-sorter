// ABOUTME: Core genetic algorithm implementation for playlist optimization
// ABOUTME: Includes fitness calculation, crossover, mutation, and 2-opt local search

package main

import (
	"context"
	"math"
	"math/rand"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/alitto/pond"
	"playlist-sorter/playlist"
)

const (
	maxDuration = 1 * time.Hour // Maximum optimization time
)

// Individual represents a candidate solution in the genetic algorithm
// with its fitness score (lower is better)
type Individual struct {
	Genes []playlist.Track // The track ordering
	Score float64          // Fitness score (lower = better)
}

// FitnessBreakdown shows the individual components contributing to fitness
type FitnessBreakdown struct {
	Harmonic      float64 // Harmonic distance penalties
	SameArtist    float64 // Same artist penalties
	SameAlbum     float64 // Same album penalties
	EnergyDelta   float64 // Energy change penalties
	BPMDelta      float64 // BPM difference penalties
	PositionBias  float64 // Low energy position bias
	Total         float64 // Sum of all components
}

// GAUpdate contains information about the current state of the genetic algorithm
type GAUpdate struct {
	Generation   int
	BestFitness  float64
	BestPlaylist []playlist.Track
	GenPerSec    float64
	Breakdown    FitnessBreakdown // Fitness breakdown
}

// SharedConfig wraps GAConfig with a mutex for thread-safe access between GA and TUI
type SharedConfig struct {
	mu     sync.RWMutex
	config GAConfig
}

// Get returns a copy of the current config (thread-safe read)
func (sc *SharedConfig) Get() GAConfig {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.config
}

// Update updates the config (thread-safe write)
func (sc *SharedConfig) Update(config GAConfig) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.config = config
}

// EdgeData stores pre-calculated base values for track transitions (without weights applied)
// Weights are applied at evaluation time to enable real-time parameter tuning
type EdgeData struct {
	HarmonicDistance int
	SameArtist       bool
	SameAlbum        bool
	EnergyDelta      float64
	BPMDelta         float64
}

// edgeDataCache stores pre-calculated base values for track transitions
// Indexed by [fromTrackIdx][toTrackIdx] for O(1) lookup
// Built once using sync.Once to avoid race conditions
var (
	edgeDataCache [][]EdgeData
	cacheOnce     sync.Once
)

// geneticSort optimizes track ordering using a genetic algorithm with context-based cancellation
//
// The algorithm works as follows:
//  1. Initialize population with random track orderings (plus current order)
//  2. For each generation:
//     a. Evaluate fitness of each candidate (lower score = better)
//     b. Sort population by fitness
//     c. Inject random immigrants to maintain diversity
//     d. Select parents: keep top 2 (elitism) + tournament selection
//     e. Create offspring via Edge Recombination Crossover (preserves good transitions)
//     f. Apply 2-opt local search to elite offspring
//     g. Mutate non-elite offspring (swaps or inversions)
//  3. Continue until context cancelled, timeout, or convergence
//  4. Return best solution found across all generations
//
// Fitness minimizes:
// - Harmonic distance between adjacent track keys (Camelot wheel)
// - Same artist/album penalties
// - Energy deltas between tracks
// - BPM differences (accounting for half/double time)
// - Position bias (prefers low-energy tracks at start)
func geneticSort(ctx context.Context, tracks []playlist.Track, sharedConfig *SharedConfig, updateChan chan<- GAUpdate) []playlist.Track {
	startTime := time.Now()
	gen := 0

	// Get initial config snapshot
	config := sharedConfig.Get()

	// Pre-calculate edge data for all track pairs (base values only, weights applied at eval time)
	buildEdgeFitnessCache(tracks)

	// Create worker pool sized to available CPUs for optimal parallelism
	pool := pond.New(runtime.NumCPU(), config.PopulationSize*2)
	defer pool.StopAndWait()

	// Initialize two populations for double buffering (avoids allocations)
	population := make([][]playlist.Track, config.PopulationSize)
	nextPopulation := make([][]playlist.Track, config.PopulationSize)

	// Keep first individual as the current playlist order (allows iterative improvement)
	population[0] = slices.Clone(tracks)
	nextPopulation[0] = make([]playlist.Track, len(tracks))

	// Initialize the rest with random orderings
	for i := 1; i < config.PopulationSize; i++ {
		population[i] = slices.Clone(tracks)
		rand.Shuffle(len(population[i]), func(a, b int) {
			population[i][a], population[i][b] = population[i][b], population[i][a]
		})
		nextPopulation[i] = make([]playlist.Track, len(tracks))
	}

	// Pre-allocate Edge Recombination Crossover buffers (reused across all crossover operations)
	numTracks := len(tracks)
	crossoverEdges := make([][]int, numTracks)
	parent1Index := make(map[string]int, numTracks)
	parent2Index := make(map[string]int, numTracks)
	seenEdges := make([]map[int]bool, numTracks)
	usedTracks := make([]bool, numTracks)
	for i := 0; i < numTracks; i++ {
		crossoverEdges[i] = make([]int, 0, 4)
		seenEdges[i] = make(map[int]bool, 4)
	}

	// Track best individual across all generations
	var bestIndividual []playlist.Track
	var bestFitness float64 = math.MaxFloat64
	var generationsWithoutImprovement int = 0

	// Reusable slice for scored population
	scoredPopulation := make([]Individual, len(population))

	// For generation speed calculation
	lastGenTime := startTime
	lastGenCount := 0

	// Ensure channel is closed exactly once
	var closeOnce sync.Once
	closeChannel := func() {
		if updateChan != nil {
			closeOnce.Do(func() {
				close(updateChan)
			})
		}
	}
	defer closeChannel()

	// Main optimization loop - runs until context cancelled or maxDuration
loop:
	for {

		// Check for cancellation or timeout
		select {
		case <-ctx.Done():
			break loop
		default:
			if time.Since(startTime) >= maxDuration {
				break loop
			}
		}

		// Get current config for this generation
		config = sharedConfig.Get()

		// Evaluate fitness for each individual playlist (parallelized with worker pool)
		group := pool.Group()
		for i := range population {
			idx := i
			group.Submit(func() {
				scoredPopulation[idx] = Individual{
					Genes: population[idx],
					Score: calculateFitness(population[idx], config),
				}
			})
		}
		group.Wait()

		// Sort population from lowest score (better fit) to highest (worse fit)
		slices.SortFunc(scoredPopulation, func(a Individual, b Individual) int {
			return int(a.Score - b.Score)
		})

		// Elitism-based immigration: Replace worst 5% with mutated elite individuals
		immigrantCount := int(float64(config.PopulationSize) * config.ImmigrationRate)
		for i := 0; i < immigrantCount; i++ {
			worstIdx := len(scoredPopulation) - 1 - i
			scoredPopulation[worstIdx].Genes = slices.Clone(scoredPopulation[0].Genes)
			numSwaps := len(tracks) / 10
			if numSwaps < 3 {
				numSwaps = 3
			}
			for s := 0; s < numSwaps; s++ {
				a := rand.Intn(len(scoredPopulation[worstIdx].Genes))
				b := rand.Intn(len(scoredPopulation[worstIdx].Genes))
				scoredPopulation[worstIdx].Genes[a], scoredPopulation[worstIdx].Genes[b] = scoredPopulation[worstIdx].Genes[b], scoredPopulation[worstIdx].Genes[a]
			}
			scoredPopulation[worstIdx].Score = math.MaxFloat64
		}

		var parents [][]playlist.Track

		// Keep top 2 as elite parents
		parents = append(parents, scoredPopulation[0].Genes)
		parents = append(parents, scoredPopulation[1].Genes)

		// Fill the rest with tournament selection
		for i := 2; i < len(scoredPopulation); i++ {
			bestIdx := rand.Intn(len(scoredPopulation))
			bestScore := scoredPopulation[bestIdx].Score
			for j := 1; j < config.TournamentSize; j++ {
				idx := rand.Intn(len(scoredPopulation))
				if scoredPopulation[idx].Score < bestScore {
					bestIdx = idx
					bestScore = scoredPopulation[idx].Score
				}
			}
			parents = append(parents, scoredPopulation[bestIdx].Genes)
		}

		// Buffer swap
		children := nextPopulation

		// Keep top 2 unchanged
		copy(children[0], parents[0])
		copy(children[1], parents[1])

		// Create offspring through crossover
		for i := 2; i < len(parents)-1; i += 2 {
			edgeRecombinationCrossover(children[i], parents[i], parents[i+1], crossoverEdges, parent1Index, parent2Index, seenEdges, usedTracks)
			edgeRecombinationCrossover(children[i+1], parents[i+1], parents[i], crossoverEdges, parent1Index, parent2Index, seenEdges, usedTracks)
		}

		// Handle odd population size
		if len(parents)%2 == 1 {
			edgeRecombinationCrossover(children[len(parents)-1], parents[len(parents)-1], parents[0], crossoverEdges, parent1Index, parent2Index, seenEdges, usedTracks)
		}

		// Apply 2-opt local search to elite children
		topCount := int(float64(len(children)) * config.ElitePercentage)
		if topCount < 2 {
			topCount = 2
		}
		group = pool.Group()
		for i := 0; i < topCount; i++ {
			idx := i
			group.Submit(func() {
				twoOptImprove(children[idx], config)
			})
		}
		group.Wait()

		// Calculate adaptive mutation rate
		mutationRate := config.MaxMutationRate - (float64(generationsWithoutImprovement)/config.MutationDecayGen)*(config.MaxMutationRate-config.MinMutationRate)
		if mutationRate < config.MinMutationRate {
			mutationRate = config.MinMutationRate
		}

		// Mutate offspring (skip top 2)
		for i := 2; i < len(children); i++ {
			if rand.Float64() < mutationRate {
				if rand.Float64() < 0.5 {
					// Swap mutation
					numSwaps := config.MinSwapMutations + rand.Intn(config.MaxSwapMutations-config.MinSwapMutations+1)
					for s := 0; s < numSwaps; s++ {
						a := rand.Intn(len(children[i]))
						b := rand.Intn(len(children[i]))
						children[i][a], children[i][b] = children[i][b], children[i][a]
					}
				} else {
					// Inversion mutation
					start := rand.Intn(len(children[i]))
					end := rand.Intn(len(children[i]))
					if start > end {
						start, end = end, start
					}
					for start < end {
						children[i][start], children[i][end] = children[i][end], children[i][start]
						start++
						end--
					}
				}
			}
		}

		// Swap populations (double buffering)
		population, nextPopulation = nextPopulation, population

		// Track best individual
		// Re-evaluate bestFitness with current config to allow fair comparison when config changes
		if bestIndividual != nil {
			bestFitness = calculateFitness(bestIndividual, config)
		}
		if scoredPopulation[0].Score < bestFitness {
			bestFitness = scoredPopulation[0].Score
			bestIndividual = slices.Clone(scoredPopulation[0].Genes)
			generationsWithoutImprovement = 0
		} else {
			generationsWithoutImprovement++
		}

		// Send update every 10 generations
		if gen%10 == 0 && updateChan != nil {
			// Calculate generation speed
			now := time.Now()
			elapsed := now.Sub(lastGenTime).Seconds()
			genPerSec := 0.0
			if elapsed > 0 {
				genPerSec = float64(gen-lastGenCount) / elapsed
			}
			lastGenTime = now
			lastGenCount = gen

			// Send the all-time best individual (across all generations)
			// Re-evaluate with current config for accurate fitness display and breakdown
			breakdown := calculateFitnessWithBreakdown(bestIndividual, config)

			select {
			case updateChan <- GAUpdate{
				Generation:   gen,
				BestFitness:  breakdown.Total,
				BestPlaylist: slices.Clone(bestIndividual),
				GenPerSec:    genPerSec,
				Breakdown:    breakdown,
			}:
			default:
				// Don't block if channel is full
			}
		}

		gen++
	}

	// Channel will be closed by deferred closeChannel()
	// Return best individual found
	return bestIndividual
}

// calculateFitness computes the fitness score for a given playlist ordering
func calculateFitness(individual []playlist.Track, config GAConfig) float64 {
	breakdown := calculateFitnessWithBreakdown(individual, config)
	return breakdown.Total
}

// calculateFitnessWithBreakdown computes fitness and returns detailed breakdown
func calculateFitnessWithBreakdown(individual []playlist.Track, config GAConfig) FitnessBreakdown {
	return segmentFitnessWithBreakdown(individual, 0, len(individual)-1, config)
}

// buildEdgeFitnessCache pre-calculates base values for all possible track pairs
// Weights are NOT cached - they're applied at evaluation time for live parameter updates
// Uses sync.Once to ensure cache is built exactly once, avoiding race conditions
// Also assigns Index values to tracks (safe because it's inside sync.Once)
func buildEdgeFitnessCache(tracks []playlist.Track) {
	cacheOnce.Do(func() {
		n := len(tracks)

		// Assign Index values to tracks (safe - happens exactly once before any concurrent access)
		for i := range tracks {
			tracks[i].Index = i
		}

		// Allocate 2D array for edge data
		edgeDataCache = make([][]EdgeData, n)
		for i := range edgeDataCache {
			edgeDataCache[i] = make([]EdgeData, n)
		}

		// Pre-calculate base values for all track pairs
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				if i == j {
					continue // Skip self-edges
				}

				t1, t2 := &tracks[i], &tracks[j]

				// Harmonic distance (base value)
				harmonicDist := playlist.HarmonicDistanceParsed(t1.ParsedKey, t2.ParsedKey)

				// Artist/album matches (boolean)
				sameArtist := t1.Artist == t2.Artist
				sameAlbum := t1.Album == t2.Album

				// Energy delta (base value)
				energyDelta := math.Abs(float64(t1.Energy - t2.Energy))

				// BPM delta (base value, accounting for half/double time)
				bpmDelta := 0.0
				if t1.BPM > 0 && t2.BPM > 0 {
					bpm1, bpm2 := t1.BPM, t2.BPM
					minBPMDistance := math.Abs(bpm1 - bpm2)
					if d := math.Abs(bpm1*0.5 - bpm2); d < minBPMDistance {
						minBPMDistance = d
					}
					if d := math.Abs(bpm1 - bpm2*0.5); d < minBPMDistance {
						minBPMDistance = d
					}
					if d := math.Abs(bpm1*2.0 - bpm2); d < minBPMDistance {
						minBPMDistance = d
					}
					if d := math.Abs(bpm1 - bpm2*2.0); d < minBPMDistance {
						minBPMDistance = d
					}
					bpmDelta = minBPMDistance
				}

				// Store base values in cache (no weights applied)
				edgeDataCache[i][j] = EdgeData{
					HarmonicDistance: harmonicDist,
					SameArtist:       sameArtist,
					SameAlbum:        sameAlbum,
					EnergyDelta:      energyDelta,
					BPMDelta:         bpmDelta,
				}
			}
		}
	})
}

// segmentFitness calculates fitness contribution for a track segment
// Reads base values from cache and applies current config weights at evaluation time
func segmentFitness(tracks []playlist.Track, start, end int, config GAConfig) float64 {
	breakdown := segmentFitnessWithBreakdown(tracks, start, end, config)
	return breakdown.Total
}

// segmentFitnessWithBreakdown calculates fitness and returns breakdown of components
func segmentFitnessWithBreakdown(tracks []playlist.Track, start, end int, config GAConfig) FitnessBreakdown {
	var breakdown FitnessBreakdown
	biasThreshold := int(float64(len(tracks)) * config.LowEnergyBiasPortion)

	// Calculate fitness for the segment [start:end+1]
	for j := start; j <= end; j++ {
		// Add edge fitness with current weights
		if j > 0 {
			// Use pre-assigned Index values for O(1) cache lookup
			idx1 := tracks[j-1].Index
			idx2 := tracks[j].Index
			edge := edgeDataCache[idx1][idx2]

			// Apply weights at evaluation time (not cached) and track each component
			harmonicPenalty := float64(edge.HarmonicDistance) * config.HarmonicWeight
			breakdown.Harmonic += harmonicPenalty

			if edge.SameArtist {
				breakdown.SameArtist += config.SameArtistPenalty
			}
			if edge.SameAlbum {
				breakdown.SameAlbum += config.SameAlbumPenalty
			}

			energyPenalty := edge.EnergyDelta * config.EnergyDeltaWeight
			breakdown.EnergyDelta += energyPenalty

			bpmPenalty := edge.BPMDelta * config.BPMDeltaWeight
			breakdown.BPMDelta += bpmPenalty
		}

		// Position-based energy penalty
		if j < biasThreshold {
			positionWeight := 1.0 - float64(j)/float64(biasThreshold)
			energyPositionPenalty := float64(tracks[j].Energy) * positionWeight * config.LowEnergyBiasWeight
			breakdown.PositionBias += energyPositionPenalty
		}
	}

	// Calculate total
	breakdown.Total = breakdown.Harmonic + breakdown.SameArtist + breakdown.SameAlbum +
		breakdown.EnergyDelta + breakdown.BPMDelta + breakdown.PositionBias

	return breakdown
}

// twoOptImprove applies 2-opt local search to polish a playlist
func twoOptImprove(tracks []playlist.Track, config GAConfig) {
	n := len(tracks)
	improved := true

	// Don't look bits: track positions that recently failed to improve
	dontLook := make([]bool, n)

	// Calculate initial full fitness once
	currentFitness := calculateFitness(tracks, config)

	// Keep iterating until no more improvements found
	for improved {
		improved = false

		for i := 0; i < n-2; i++ {
			if dontLook[i] {
				continue
			}

			positionImproved := false
			for j := i + 2; j < n; j++ {
				endPos := j + 1
				if endPos >= n {
					endPos = n - 1
				}
				oldSegmentFitness := segmentFitness(tracks, i, endPos, config)

				// Try reversing [i+1:j] and check if it improves fitness
				reverseSegment(tracks, i+1, j)
				newSegmentFitness := segmentFitness(tracks, i, endPos, config)

				delta := newSegmentFitness - oldSegmentFitness
				newFitness := currentFitness + delta

				if newFitness < currentFitness {
					// Keep the reversal
					currentFitness = newFitness
					improved = true
					positionImproved = true
					// Reset all don't-look bits
					for k := range dontLook {
						dontLook[k] = false
					}
				} else {
					// Undo the reversal
					reverseSegment(tracks, i+1, j)
				}
			}

			if !positionImproved {
				dontLook[i] = true
			}
		}
	}
}


// reverseSegment reverses tracks[start:end+1] in place
func reverseSegment(tracks []playlist.Track, start, end int) {
	for start < end {
		tracks[start], tracks[end] = tracks[end], tracks[start]
		start++
		end--
	}
}

// edgeRecombinationCrossover creates offspring that preserve good track transitions
func edgeRecombinationCrossover(dst, parent1, parent2 []playlist.Track, edges [][]int, p1Index, p2Index map[string]int, seen []map[int]bool, used []bool) {
	numTracks := len(parent1)

	// Build edge table
	for i := 0; i < numTracks; i++ {
		p1Index[parent1[i].Path] = i
		p2Index[parent2[i].Path] = i
	}

	// Reset edges and add parent1 edges
	for i := 0; i < numTracks; i++ {
		edges[i] = edges[i][:0]
		for k := range seen[i] {
			delete(seen[i], k)
		}
		if i > 0 {
			edges[i] = append(edges[i], i-1)
			seen[i][i-1] = true
		}
		if i < numTracks-1 {
			edges[i] = append(edges[i], i+1)
			seen[i][i+1] = true
		}
	}

	// Add edges from parent2
	for i1, track1 := range parent1 {
		i2 := p2Index[track1.Path]

		if i2 > 0 {
			j1 := p1Index[parent2[i2-1].Path]
			if !seen[i1][j1] {
				edges[i1] = append(edges[i1], j1)
				seen[i1][j1] = true
			}
		}

		if i2 < numTracks-1 {
			j1 := p1Index[parent2[i2+1].Path]
			if !seen[i1][j1] {
				edges[i1] = append(edges[i1], j1)
				seen[i1][j1] = true
			}
		}
	}

	// Clear used array
	for i := range used {
		used[i] = false
	}

	// Start with random track
	currentIdx := rand.Intn(numTracks)
	dst[0] = parent1[currentIdx]
	used[currentIdx] = true

	// Build offspring by selecting best neighbors
	for pos := 1; pos < numTracks; pos++ {
		nextIdx := -1
		minEdges := math.MaxInt

		for _, neighbor := range edges[currentIdx] {
			if used[neighbor] {
				continue
			}

			edgeCount := 0
			for _, e := range edges[neighbor] {
				if !used[e] {
					edgeCount++
				}
			}

			if edgeCount < minEdges {
				minEdges = edgeCount
				nextIdx = neighbor
			}
		}

		if nextIdx == -1 {
			// No valid neighbors - pick random unused track
			unused := make([]int, 0, len(used))
			for i, u := range used {
				if !u {
					unused = append(unused, i)
				}
			}
			if len(unused) > 0 {
				nextIdx = unused[rand.Intn(len(unused))]
			}
			if nextIdx == -1 {
				break
			}
		}

		dst[pos] = parent1[nextIdx]
		used[nextIdx] = true
		currentIdx = nextIdx
	}
}
