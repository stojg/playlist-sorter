// ABOUTME: Core genetic algorithm implementation for playlist optimization
// ABOUTME: Includes fitness calculation, crossover, mutation, and 2-opt local search

package main

import (
	"context"
	"log"
	"math"
	"math/rand/v2"
	"os"
	"runtime"
	"slices"
	"sync"
	"time"

	"playlist-sorter/playlist"

	"github.com/alitto/pond"
)

const (
	maxDuration = 1 * time.Hour // Maximum optimization time

	// Genetic algorithm constants
	maxMutationRate    = 0.3
	minMutationRate    = 0.1
	mutationDecayGen   = 100.0
	minSwapMutations   = 2
	maxSwapMutations   = 5
	immigrationRate    = 0.15
	elitePercentage    = 0.03
	tournamentSize     = 3
	twoOptStartGen     = 50  // Generation to start applying 2-opt
	twoOptIntervalGens = 100 // Apply 2-opt every N generations after start
)

// Debug logger - writes to file in visual mode
var debugLog *log.Logger

// InitDebugLog initializes debug logging to a file
func InitDebugLog(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	debugLog = log.New(f, "", log.Ltime|log.Lmicroseconds)
	return nil
}

// debugf logs debug messages to file if debug logger is enabled
func debugf(format string, args ...interface{}) {
	if debugLog != nil {
		debugLog.Printf(format, args...)
	}
}

// Individual represents a candidate solution in the genetic algorithm
// with its fitness score (lower is better)
type Individual struct {
	Genes []playlist.Track // The track ordering
	Score float64          // Fitness score (lower = better)
}

// FitnessBreakdown shows the individual components contributing to fitness
type FitnessBreakdown struct {
	Harmonic     float64 // Harmonic distance penalties
	SameArtist   float64 // Same artist penalties
	SameAlbum    float64 // Same album penalties
	EnergyDelta  float64 // Energy change penalties
	BPMDelta     float64 // BPM difference penalties
	GenreChange  float64 // Genre change/clustering penalty (signed weight)
	PositionBias float64 // Low energy position bias
	Total        float64 // Sum of all components
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
	GenreDifference  float64 // Genre similarity: 0.0 = same, 1.0 = completely different
}

// FitnessNormalizers stores maximum possible values for each fitness component
// Used to normalize components to [0,1] scale so weights have equal influence
type FitnessNormalizers struct {
	MaxHarmonic     float64 // Maximum possible harmonic distance across all transitions
	MaxSameArtist   float64 // Maximum possible same artist occurrences
	MaxSameAlbum    float64 // Maximum possible same album occurrences
	MaxEnergyDelta  float64 // Maximum possible energy delta sum
	MaxBPMDelta     float64 // Maximum possible BPM delta sum
	MaxPositionBias float64 // Maximum possible position bias penalty
	MaxGenreChange  float64 // Maximum possible genre change penalty
}

// edgeDataCache stores pre-calculated base values for track transitions
// Indexed by [fromTrackIdx][toTrackIdx] for O(1) lookup
// Built once using sync.Once to avoid race conditions
var (
	edgeDataCache [][]EdgeData
	normalizers   FitnessNormalizers
	cacheOnce     sync.Once
)

// geneticSort optimizes track ordering using a genetic algorithm with context-based cancellation
//
// The algorithm works as follows:
//  1. Initialize population with random track orderings (plus current order)
//  2. For each generation:
//     a. Evaluate fitness of each candidate (lower score = better)
//     b. Sort population by fitness (best to worst)
//     c. Apply 2-opt local search to elite solutions (periodically: gen 50, then every 100 gens)
//     d. Track best individual across all generations
//     e. Inject random immigrants by replacing worst individuals with mutated copies of best
//     f. Select parents: keep top 2 (elitism) + tournament selection for rest
//     g. Create offspring via Order Crossover (maintains relative ordering from parents)
//     h. Mutate offspring (but skip top 2 elite): swap or inversion mutations
//  3. Continue until context cancelled, timeout (1 hour), or convergence
//  4. Return best solution found across all generations
//
// Fitness minimizes (all normalized to [0,1] scale before applying weights):
// - Harmonic distance between adjacent track keys (Camelot wheel)
// - Same artist/album penalties (consecutive tracks)
// - Energy deltas between tracks
// - BPM differences (accounting for half/double time mixing)
// - Position bias (prefers low-energy tracks at start of playlist)
// - Genre changes (optional, signed weight: positive=cluster, negative=spread)
func geneticSort(ctx context.Context, tracks []playlist.Track, sharedConfig *SharedConfig, updateChan chan<- GAUpdate) []playlist.Track {
	startTime := time.Now()
	gen := 0
	const populationSize = 100
	genesLen := len(tracks) // Cache length (all Genes slices are pre-allocated to this size)

	// Get the initial config snapshot
	config := sharedConfig.Get()

	// Pre-calculate fitness score for all possible track pairs so we don't do this in tight loops
	buildEdgeFitnessCache(tracks)

	// Create worker pool sized to available CPUs for optimal parallelism
	pool := pond.New(runtime.NumCPU(), populationSize*2)
	defer pool.StopAndWait()

	// Initialize two generation buffers (avoids allocations and prevents parent corruption during crossover)
	currentGen := make([][]playlist.Track, populationSize)
	nextGen := make([][]playlist.Track, populationSize)

	// Seed population with greedy solutions for faster convergence
	// These provide good starting points that already optimize one constraint well
	currentGen[0] = slices.Clone(tracks) // Current order
	nextGen[0] = make([]playlist.Track, genesLen)

	// Individual 1: Sort by energy (ascending = smooth flow)
	if populationSize > 1 {
		currentGen[1] = slices.Clone(tracks)
		slices.SortFunc(currentGen[1], func(a, b playlist.Track) int {
			return a.Energy - b.Energy
		})
		nextGen[1] = make([]playlist.Track, genesLen)
	}

	// Individual 2: Sort by BPM (ascending)
	if populationSize > 2 {
		currentGen[2] = slices.Clone(tracks)
		slices.SortFunc(currentGen[2], func(a, b playlist.Track) int {
			if a.BPM < b.BPM {
				return -1
			} else if a.BPM > b.BPM {
				return 1
			}
			return 0
		})
		nextGen[2] = make([]playlist.Track, genesLen)
	}

	// Individual 3: Sort by Camelot key (1A, 2A, ..., 12A, 1B, ..., 12B)
	if populationSize > 3 {
		currentGen[3] = slices.Clone(tracks)
		slices.SortFunc(currentGen[3], func(a, b playlist.Track) int {
			if a.ParsedKey == nil && b.ParsedKey == nil {
				return 0
			}
			if a.ParsedKey == nil {
				return 1
			}
			if b.ParsedKey == nil {
				return -1
			}
			// Sort by letter first (A before B), then by number
			if a.ParsedKey.Letter != b.ParsedKey.Letter {
				return int(a.ParsedKey.Letter - b.ParsedKey.Letter)
			}
			return a.ParsedKey.Number - b.ParsedKey.Number
		})
		nextGen[3] = make([]playlist.Track, genesLen)
	}

	// Initialize the rest with random orderings
	for i := 4; i < populationSize; i++ {
		currentGen[i] = slices.Clone(tracks)
		rand.Shuffle(len(currentGen[i]), func(a, b int) {
			currentGen[i][a], currentGen[i][b] = currentGen[i][b], currentGen[i][a]
		})
		nextGen[i] = make([]playlist.Track, genesLen)
	}

	// Track best individual across all generations
	var bestIndividual []playlist.Track
	var bestFitness float64 = math.MaxFloat64
	var generationsWithoutImprovement int = 0

	// Reusable slice for scored population (pre-allocate Genes to avoid allocations in hot loop)
	scoredPopulation := make([]Individual, populationSize)
	for i := range scoredPopulation {
		scoredPopulation[i].Genes = make([]playlist.Track, genesLen)
	}

	// For generation speed calculation
	lastGenTime := startTime
	lastGenCount := 0

	// Reusable map buffer for orderCrossover to avoid allocations
	// Sized to track presence of all tracks in playlist
	presentMap := make(map[string]bool, genesLen)

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
		debugf("[GA] Getting config for gen %d", gen)
		config = sharedConfig.Get()
		debugf("[GA] Config retrieved - Genre Weight: %.2f", config.GenreWeight)

		// Evaluate fitness for each individual playlist (parallelized with worker pool)
		debugf("[GA] Starting fitness evaluation for gen %d", gen)
		group := pool.Group()
		for i := range currentGen {
			group.Submit(func() {
				scoredPopulation[i] = Individual{Genes: currentGen[i], Score: calculateFitness(currentGen[i], config)}
			})
		}
		group.Wait()
		debugf("[GA] Fitness evaluation complete for gen %d", gen)

		// Sort population from lowest score (better fit) to highest (worse fit)
		slices.SortFunc(scoredPopulation, func(a Individual, b Individual) int {
			if a.Score < b.Score {
				return -1
			}
			if a.Score > b.Score {
				return 1
			}
			return 0
		})

		// Apply 2-opt local search to elite periodically (improves sorted population in-place)
		shouldRunTwoOpt := gen >= twoOptStartGen && (gen == twoOptStartGen || (gen-twoOptStartGen)%twoOptIntervalGens == 0)
		if shouldRunTwoOpt {
			topCount := int(float64(populationSize) * elitePercentage)
			if topCount < 2 {
				topCount = 2
			}
			debugf("[GA] Starting 2-opt for gen %d (topCount=%d)", gen, topCount)
			group = pool.Group()
			for i := 0; i < topCount; i++ {
				group.Submit(func() {
					twoOptImprove(scoredPopulation[i].Genes, config)
				})
			}
			group.Wait()
			debugf("[GA] 2-opt complete for gen %d", gen)
		}

		// Check if new best individual from children
		fitnessImproved := false
		if scoredPopulation[0].Score < bestFitness {
			bestFitness = scoredPopulation[0].Score
			bestIndividual = slices.Clone(scoredPopulation[0].Genes)
			generationsWithoutImprovement = 0
			fitnessImproved = true
		} else {
			generationsWithoutImprovement++
		}

		// Send update when fitness improves or every 10 generations
		if (fitnessImproved || gen%10 == 0) && updateChan != nil {
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

		// Now we start the genetic algorithm itself

		// Replace the worst individuals with mutated copies of the best individual
		// This introduces new genetic material while preserving good solutions
		immigrantCount := int(float64(populationSize) * immigrationRate)
		immigrantSwaps := genesLen / 10
		if immigrantSwaps < 3 {
			immigrantSwaps = 3
		}

		for i := 0; i < immigrantCount; i++ {
			worstIdx := len(scoredPopulation) - 1 - i
			// Copy genes from the best individual
			copy(scoredPopulation[worstIdx].Genes, scoredPopulation[0].Genes)
			// Apply random swaps to create variation
			for s := 0; s < immigrantSwaps; s++ {
				a := rand.IntN(genesLen)
				b := rand.IntN(genesLen)
				scoredPopulation[worstIdx].Genes[a], scoredPopulation[worstIdx].Genes[b] = scoredPopulation[worstIdx].Genes[b], scoredPopulation[worstIdx].Genes[a]
			}
			// Re-evaluate fitness after mutation
			scoredPopulation[worstIdx].Score = calculateFitness(scoredPopulation[worstIdx].Genes, config)
		}

		parents := make([][]playlist.Track, populationSize)

		// make the top two from the current population parents
		parents[0] = scoredPopulation[0].Genes
		parents[1] = scoredPopulation[1].Genes

		// Fill the rest of the population with a tournament selection
		for i := 2; i < len(scoredPopulation); i++ {
			// grab a random individual from the tournament
			bestIdx := rand.IntN(len(scoredPopulation))
			// keep the best individual from the tournament
			bestScore := scoredPopulation[bestIdx].Score
			// check the best individual against tournamentSize other random individuals and keep the best
			for j := 1; j < tournamentSize; j++ {
				idx := rand.IntN(len(scoredPopulation))
				if scoredPopulation[idx].Score < bestScore {
					bestIdx = idx
					bestScore = scoredPopulation[idx].Score
				}
			}
			parents[i] = scoredPopulation[bestIdx].Genes
		}

		// Keep top two elite (2-opt improved) unchanged in next generation
		copy(nextGen[0], scoredPopulation[0].Genes)
		copy(nextGen[1], scoredPopulation[1].Genes)

		// Create offspring through Order Crossover (OX)
		// Simpler and faster than Edge Recombination Crossover (ERC), with good exploration characteristics
		for i := 2; i < len(parents)-1; i += 2 {
			orderCrossover(nextGen[i], parents[i], parents[i+1], presentMap)
			orderCrossover(nextGen[i+1], parents[i+1], parents[i], presentMap)
		}
		// Handle odd population size
		if len(parents)%2 == 1 {
			orderCrossover(nextGen[len(parents)-1], parents[len(parents)-1], parents[0], presentMap)
		}

		// Calculate adaptive mutation rate (increases when stuck to escape local optima)
		mutationRate := minMutationRate + (float64(generationsWithoutImprovement)/mutationDecayGen)*(maxMutationRate-minMutationRate)
		if mutationRate > maxMutationRate {
			mutationRate = maxMutationRate
		}

		// Mutate offspring (but skip the top two individuals)
		for i := 2; i < populationSize; i++ {
			if rand.Float64() < mutationRate {
				// Choose between swap and inversion mutation (50/50 chance)
				// Uint32()&1 extracts the least significant bit: 23% faster than Float64() < 0.5
				// This runs every generation for ~98 individuals, so the speedup is significant
				if rand.Uint32()&1 == 0 {
					// Swap mutation: randomly swap 2-5 pairs of tracks
					// Good for small local changes and escaping local optima
					numSwaps := minSwapMutations + rand.IntN(maxSwapMutations-minSwapMutations+1)
					for s := 0; s < numSwaps; s++ {
						a := rand.IntN(genesLen)
						b := rand.IntN(genesLen)
						nextGen[i][a], nextGen[i][b] = nextGen[i][b], nextGen[i][a]
					}
				} else {
					// Inversion mutation: reverse a random segment of the playlist
					// More disruptive than swap, helps explore distant solutions
					start := rand.IntN(genesLen)
					end := rand.IntN(genesLen)
					if start > end {
						start, end = end, start
					}
					reverseSegment(nextGen[i], start, end)
				}
			}
		}

		// Swap generation buffers for next iteration
		currentGen, nextGen = nextGen, currentGen

		debugf("[GA] Generation %d complete", gen)
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

// calculateTheoreticalMinimum calculates the theoretical minimum fitness score
// This is NOT achievable in practice as the constraints conflict with each other
// (e.g., monotonic energy vs clustered low energy at start), but provides a lower bound
func calculateTheoreticalMinimum(tracks []playlist.Track, config GAConfig) float64 {
	n := len(tracks)
	if n == 0 {
		return 0.0
	}

	// 1. Harmonic: Best case = all tracks have perfect harmonic compatibility (distance 0)
	minHarmonic := 0.0

	// 2. Same Artist: Best case = no consecutive tracks from same artist
	minSameArtist := 0.0

	// 3. Same Album: Best case = no consecutive tracks from same album
	minSameAlbum := 0.0

	// 4. Energy Delta: Best case = tracks sorted by energy (monotonic increase/decrease)
	energies := make([]int, n)
	for i, t := range tracks {
		energies[i] = t.Energy
	}
	slices.Sort(energies)
	minEnergyDelta := 0.0
	for i := 1; i < n; i++ {
		minEnergyDelta += math.Abs(float64(energies[i] - energies[i-1]))
	}
	if normalizers.MaxEnergyDelta > 0 {
		minEnergyDelta = (minEnergyDelta / normalizers.MaxEnergyDelta) * config.EnergyDeltaWeight
	}

	// 5. BPM Delta: Best case = tracks sorted by BPM
	bpms := make([]float64, 0, n)
	for _, t := range tracks {
		if t.BPM > 0 {
			bpms = append(bpms, t.BPM)
		}
	}
	slices.Sort(bpms)
	minBPMDelta := 0.0
	for i := 1; i < len(bpms); i++ {
		minBPMDelta += math.Abs(bpms[i] - bpms[i-1])
	}
	if normalizers.MaxBPMDelta > 0 && len(bpms) > 1 {
		minBPMDelta = (minBPMDelta / normalizers.MaxBPMDelta) * config.BPMDeltaWeight
	}

	// 6. Position Bias: Best case = lowest energy tracks at start
	biasThreshold := int(float64(n) * config.LowEnergyBiasPortion)
	minPositionBias := 0.0
	for j := 0; j < biasThreshold && j < n; j++ {
		positionWeight := 1.0 - float64(j)/float64(biasThreshold)
		rawBias := float64(energies[j]) * positionWeight
		if normalizers.MaxPositionBias > 0 {
			minPositionBias += (rawBias / normalizers.MaxPositionBias) * config.LowEnergyBiasWeight
		}
	}

	// 7. Genre: Best case = 0 (either all same genre or all different, depending on weight direction)
	minGenre := 0.0

	return minHarmonic + minSameArtist + minSameAlbum +
		minEnergyDelta + minBPMDelta + minPositionBias + minGenre
}

// buildEdgeFitnessCache pre-calculates base values for all possible track pairs
// Weights are NOT cached - they're applied at evaluation time for live parameter updates
// Uses sync.Once to ensure cache is built exactly once, avoiding race conditions
// Note: Track Index values must be assigned before calling this function
func buildEdgeFitnessCache(tracks []playlist.Track) {
	cacheOnce.Do(func() {
		n := len(tracks)

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

				// Genre difference (hierarchical similarity: 0.0 = same, 1.0 = different)
				genreDiff := playlist.GenreSimilarity(t1.Genre, t2.Genre)

				// Store base values in cache (no weights applied)
				edgeDataCache[i][j] = EdgeData{
					HarmonicDistance: harmonicDist,
					SameArtist:       sameArtist,
					SameAlbum:        sameAlbum,
					EnergyDelta:      energyDelta,
					BPMDelta:         bpmDelta,
					GenreDifference:  genreDiff,
				}
			}
		}

		// Calculate normalization constants for 0-1 scaled fitness
		// These represent maximum possible values for each component across the entire playlist
		normalizers.MaxHarmonic = 12.0 * float64(n-1) // Max Camelot distance is 12

		normalizers.MaxSameArtist = float64(n - 1) // Worst case: all transitions have same artist
		normalizers.MaxSameAlbum = float64(n - 1)  // Worst case: all transitions have same album

		// Calculate max energy delta from actual track data
		minEnergy, maxEnergy := float64(tracks[0].Energy), float64(tracks[0].Energy)
		for i := 1; i < n; i++ {
			e := float64(tracks[i].Energy)
			if e < minEnergy {
				minEnergy = e
			}
			if e > maxEnergy {
				maxEnergy = e
			}
		}
		normalizers.MaxEnergyDelta = (maxEnergy - minEnergy) * float64(n-1)

		// Calculate max BPM delta from actual track data
		// Find the maximum BPM distance considering half/double time matching
		maxBPMDist := 0.0
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				if i != j && tracks[i].BPM > 0 && tracks[j].BPM > 0 {
					if edgeDataCache[i][j].BPMDelta > maxBPMDist {
						maxBPMDist = edgeDataCache[i][j].BPMDelta
					}
				}
			}
		}
		normalizers.MaxBPMDelta = maxBPMDist * float64(n-1)

		// Calculate max genre change: worst case is all transitions are completely different (1.0)
		normalizers.MaxGenreChange = float64(n - 1)

		// Calculate max position bias: maximum energy value * max position weight (1.0)
		// This normalizes each position independently, making the bias weight comparable to other weights
		// The bias portion and position weights are applied at evaluation time
		normalizers.MaxPositionBias = maxEnergy
	})
}

// segmentFitness calculates fitness contribution for a track segment
// Reads base values from cache and applies current config weights at evaluation time
func segmentFitness(tracks []playlist.Track, start, end int, config GAConfig) float64 {
	return segmentFitnessWithBreakdown(tracks, start, end, config).Total
}

// segmentFitnessWithBreakdown calculates fitness and returns breakdown of components
func segmentFitnessWithBreakdown(tracks []playlist.Track, start, end int, config GAConfig) FitnessBreakdown {
	var breakdown FitnessBreakdown
	biasThreshold := int(float64(len(tracks)) * config.LowEnergyBiasPortion)
	// Precompute genre-related values to avoid repeated checks and calculations
	genreEnabled := config.GenreWeight != 0 && normalizers.MaxGenreChange > 0
	var genreAbsWeight, genreSign float64
	if genreEnabled {
		genreAbsWeight = math.Abs(config.GenreWeight) / normalizers.MaxGenreChange
		if config.GenreWeight > 0 {
			genreSign = 1.0
		} else {
			genreSign = -1.0
		}
	}

	// Calculate fitness for the segment [start:end+1]
	for j := start; j <= end; j++ {
		// Add edge fitness with current weights
		if j > 0 {
			// Use pre-assigned Index values for O(1) cache lookup
			idx1 := tracks[j-1].Index
			idx2 := tracks[j].Index
			edge := edgeDataCache[idx1][idx2]

			// Normalize each component to [0,1] before applying weights
			// This ensures all weights have equal influence when set to same value
			normalizedHarmonic := float64(edge.HarmonicDistance) / normalizers.MaxHarmonic
			harmonicPenalty := normalizedHarmonic * config.HarmonicWeight
			breakdown.Harmonic += harmonicPenalty

			if edge.SameArtist {
				normalizedArtist := 1.0 / normalizers.MaxSameArtist
				breakdown.SameArtist += normalizedArtist * config.SameArtistPenalty
			}
			if edge.SameAlbum {
				normalizedAlbum := 1.0 / normalizers.MaxSameAlbum
				breakdown.SameAlbum += normalizedAlbum * config.SameAlbumPenalty
			}

			normalizedEnergy := edge.EnergyDelta / normalizers.MaxEnergyDelta
			energyPenalty := normalizedEnergy * config.EnergyDeltaWeight
			breakdown.EnergyDelta += energyPenalty

			normalizedBPM := edge.BPMDelta / normalizers.MaxBPMDelta
			bpmPenalty := normalizedBPM * config.BPMDeltaWeight
			breakdown.BPMDelta += bpmPenalty

			// Genre penalty: signed weight controls clustering vs spreading
			if genreEnabled {
				// Positive weight: penalize changes (clustering)
				// Negative weight: penalize same genre (spreading)
				rawPenalty := edge.GenreDifference
				if genreSign < 0 {
					rawPenalty = 1.0 - rawPenalty
				}
				breakdown.GenreChange += rawPenalty * genreAbsWeight
			}
		}

		// Position-based energy penalty
		if j < biasThreshold {
			positionWeight := 1.0 - float64(j)/float64(biasThreshold)
			rawPositionBias := float64(tracks[j].Energy) * positionWeight
			normalizedPositionBias := rawPositionBias / normalizers.MaxPositionBias
			energyPositionPenalty := normalizedPositionBias * config.LowEnergyBiasWeight
			breakdown.PositionBias += energyPositionPenalty
		}
	}

	// Calculate total
	breakdown.Total = breakdown.Harmonic + breakdown.SameArtist + breakdown.SameAlbum +
		breakdown.EnergyDelta + breakdown.BPMDelta + breakdown.PositionBias + breakdown.GenreChange

	return breakdown
}

// twoOptImprove applies 2-opt local search to polish a playlist ordering
//
// 2-opt is a classic local search algorithm originally designed for the Traveling Salesman Problem (TSP).
// It works by systematically testing segment reversals to find local fitness improvements.
//
// Algorithm:
//  1. For each position i in the playlist
//  2. For each position j > i
//  3. Try reversing the segment [i:j] (flip the order of tracks in that range)
//  4. If reversal improves fitness, keep it; otherwise, undo it
//  5. Repeat until no improvements are found (local optimum reached)
//
// Example: Playlist [A, B, C, D, E] with i=1, j=3
//
//	Before: A, [B, C, D], E
//	After:  A, [D, C, B], E  (reversed middle segment)
//
// Performance optimizations:
//   - Delta evaluation: Only recalculates fitness for the changed segment [i:endPos]
//     instead of the entire playlist. This is O(segment_size) vs O(playlist_size).
//   - Don't look bits: Tracks positions that failed to produce improvements and skips them
//     on subsequent passes. Resets when any improvement is found (positions become "active" again).
//   - Epsilon threshold (1e-10): Prevents floating point oscillations where tiny precision
//     errors cause the algorithm to flip between two equivalent solutions infinitely.
//   - Safety limit (1000 iterations): Guards against infinite loops from numerical issues.
//
// Usage in GA:
//
//	Applied to elite solutions (top 3% of population) periodically during evolution:
//	  - First applied at generation 50
//	  - Then every 100 generations thereafter
//	This balances exploration (GA) with exploitation (local search).
//
// Effectiveness:
//
//	2-opt is particularly effective for playlist optimization because:
//	  - Track orderings have strong locality (nearby tracks influence each other's fitness)
//	  - Reversing segments can fix "crossed" transitions (e.g., 8A→5A→9A becomes 8A→9A→5A)
//	  - Complementary to crossover/mutation which provide global exploration
//
// Time complexity: O(n²) per iteration, where n = playlist length
// Space complexity: O(n) for don't-look bits
func twoOptImprove(tracks []playlist.Track, config GAConfig) {
	n := len(tracks)

	// Don't look bits: track positions that recently failed to improve
	dontLook := make([]bool, n)

	// Calculate initial full fitness once
	currentFitness := calculateFitness(tracks, config)

	// Safety limit to prevent infinite loops from floating point issues
	const maxIterations = 1000
	const epsilon = 1e-10
	iteration := 0

	// Keep iterating until no more improvements found
	improved := true
	for improved && iteration < maxIterations {
		improved = false
		iteration++

		// For each position i in the playlist (but the last)
		for i := 0; i < n-1; i++ {
			if dontLook[i] {
				continue
			}
			positionImproved := false

			// and for each position after i (but before the end)
			for j := i + 1; j < n; j++ {
				// endPos = j+1 to include the edge transition at tracks[j]→tracks[j+1]
				endPos := j + 1
				if endPos >= n {
					endPos = n - 1
				}

				// Calculate fitness for the segment [i:endPos]
				oldSegmentFitness := segmentFitness(tracks, i, endPos, config)

				// Reverse segment [i,j] (inclusive), then re-evaluate fitness for [i,endPos]
				reverseSegment(tracks, i, j)
				newSegmentFitness := segmentFitness(tracks, i, endPos, config)

				newFitness := currentFitness + newSegmentFitness - oldSegmentFitness

				// If no improvement, undo the reversal and try next segment
				// Use epsilon threshold to avoid accepting tiny floating point differences
				if newFitness >= currentFitness-epsilon {
					reverseSegment(tracks, i, j)
					continue
				}

				// Improvement found - keep the reversal
				currentFitness = newFitness
				improved = true
				positionImproved = true
				clear(dontLook)
			}

			if !positionImproved {
				dontLook[i] = true
			}
		}
	}

	// Log if we hit the iteration limit
	if iteration >= maxIterations {
		debugf("[2-OPT] Hit max iterations (%d) - possible infinite loop prevented", maxIterations)
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

// orderCrossover (OX) creates offspring by preserving order from parents
// This is more exploratory than Edge Recombination Crossover, allowing better escape from local optima
// Algorithm:
//  1. Select random substring from parent1 and copy to offspring
//  2. Fill remaining positions with tracks from parent2 in order, skipping those already present
//
// The present map is passed in as a reusable buffer to avoid allocations.
// The function clears the map at the beginning, so it can be reused across calls.
func orderCrossover(dst, parent1, parent2 []playlist.Track, present map[string]bool) {
	numTracks := len(parent1)

	// Clear the map from previous use
	clear(present)

	// Select two random cut points
	cut1 := rand.IntN(numTracks)
	cut2 := rand.IntN(numTracks)
	if cut1 > cut2 {
		cut1, cut2 = cut2, cut1
	}

	// Copy substring from parent1 to offspring and track present tracks
	for i := cut1; i <= cut2; i++ {
		dst[i] = parent1[i]
		present[parent1[i].Path] = true
	}

	// Fill remaining positions with tracks from parent2 in order
	dstIdx := (cut2 + 1) % numTracks
	for i := 0; i < numTracks; i++ {
		parent2Idx := (cut2 + 1 + i) % numTracks
		if !present[parent2[parent2Idx].Path] {
			dst[dstIdx] = parent2[parent2Idx]
			dstIdx = (dstIdx + 1) % numTracks
		}
	}
}
