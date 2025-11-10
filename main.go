package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"slices"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/alitto/pond"
	"playlist-sorter/playlist"
)

// Genetic algorithm parameters
const (
	populationSize  = 100
	maxDuration     = 1 * time.Hour
	immigrationRate = 0.05 // 5% random immigration per generation

	// Adaptive mutation parameters: decay from high exploration to fine-tuning
	maxMutationRate  = 0.3   // Initial high exploration rate
	minMutationRate  = 0.1   // Final fine-tuning rate
	mutationDecayGen = 100.0 // Generations to decay from max to min
	lowEnergyBiasPortion = 0.2  // Bias first 20% of playlist towards low energy
	lowEnergyBiasWeight  = 10.0 // Weight for energy position penalty

	// Fitness penalty weights
	sameArtistPenalty = 5.0  // Penalty for consecutive tracks by same artist
	sameAlbumPenalty  = 2.0  // Penalty for consecutive tracks from same album
	energyDeltaWeight = 3.0  // Weight for energy level changes between tracks
	bpmDeltaWeight    = 0.25 // Weight for BPM differences

	// Selection and local search parameters
	tournamentSize   = 3   // Number of candidates in tournament selection
	elitePercentage  = 0.1 // Top 10% of population gets local search optimization
	minSwapMutations = 2   // Minimum number of swaps in swap mutation
	maxSwapMutations = 5   // Maximum number of swaps in swap mutation
)

func main() {
	// Define profiling flags
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to file")
	memprofile := flag.String("memprofile", "", "write memory profile to file")
	flag.Parse()

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

	fmt.Printf("Reading playlist: %s\n", playlistPath)

	// Load playlist with metadata from beets
	tracks, err := playlist.LoadPlaylistWithMetadata(playlistPath)
	if err != nil {
		log.Fatalf("Failed to load playlist: %v", err)
	}

	// Set up signal handling for Ctrl+C
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	fmt.Println("\nOptimizing playlist... (press Ctrl+C to stop early, or wait up to 1 hour)")

	sortedTracks := geneticSort(tracks, stop)

	// Show sorted playlist with tabwriter
	fmt.Println("\nSorted playlist:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "#\tKey\tBPM\tEnergy\tTitle\tArtist\tAlbum"); err != nil {
		log.Printf("Warning: failed to write header: %v", err)
	}
	if _, err := fmt.Fprintln(w, "---\t---\t---\t------\t-----\t------\t-----"); err != nil {
		log.Printf("Warning: failed to write separator: %v", err)
	}

	for i, track := range sortedTracks {
		if _, err := fmt.Fprintf(w, "%d\t%s\t%.0f\t%d\t%s\t%s\t%s\n",
			i+1,
			track.Key,
			track.BPM,
			track.Energy,
			truncate(track.Title, 30),
			truncate(track.Artist, 25),
			truncate(track.Album, 25),
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

// Individual represents a candidate solution in the genetic algorithm
// with its fitness score (lower is better)
type Individual struct {
	Genes []playlist.Track // The track ordering
	Score float64          // Fitness score (lower = better)
}

// geneticSort optimizes track ordering using a genetic algorithm
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
//  3. Continue until timeout, Ctrl+C, or convergence
//  4. Return best solution found across all generations
//
// Fitness minimizes:
// - Harmonic distance between adjacent track keys (Camelot wheel)
// - Same artist/album penalties
// - Energy deltas between tracks
// - BPM differences (accounting for half/double time)
// - Position bias (prefers low-energy tracks at start)
func geneticSort(tracks []playlist.Track, stop <-chan os.Signal) []playlist.Track {
	startTime := time.Now()
	gen := 0

	// Pre-calculate edge fitness for all track pairs
	buildEdgeFitnessCache(tracks)

	// Create worker pool sized to available CPUs for optimal parallelism
	pool := pond.New(runtime.NumCPU(), populationSize*2)
	defer pool.StopAndWait()

	// Initialize two populations for double buffering (avoids allocations)
	population := make([][]playlist.Track, populationSize)
	nextPopulation := make([][]playlist.Track, populationSize)

	// Keep first individual as the current playlist order (allows iterative improvement)
	population[0] = slices.Clone(tracks)
	nextPopulation[0] = make([]playlist.Track, len(tracks))

	// Initialize the rest with random orderings
	for i := 1; i < populationSize; i++ {
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

	var previousBestFitness float64 = math.MaxFloat64
	var lastPrintedGen int = -1

	// Track best individual across all generations
	var bestIndividual []playlist.Track
	var bestFitness float64 = math.MaxFloat64
	var generationsWithoutImprovement int = 0

	// Reusable slice for scored population
	scoredPopulation := make([]Individual, len(population))

	// Status line animation and ticker
	spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinnerIdx := 0
	statusTicker := time.NewTicker(1 * time.Second)
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
	printStatus := func() {
		elapsed := time.Since(startTime)
		fmt.Printf("\r%s Gen %d %s     ", formatElapsed(elapsed), gen, spinnerFrames[spinnerIdx])
		spinnerIdx = (spinnerIdx + 1) % len(spinnerFrames)
	}

	// Main optimization loop - runs until Ctrl+C or maxDuration
loop:
	for {
		// Check for stop signal, timeout, or status update
		select {
		case <-stop:
			fmt.Print("\r\033[K") // Clear status line
			fmt.Println("\nStopping optimization early (Ctrl+C pressed)...")
			break loop
		case <-statusTicker.C:
			printStatus()
			continue // Skip to next iteration to update status
		default:
			if time.Since(startTime) >= maxDuration {
				fmt.Print("\r\033[K") // Clear status line
				fmt.Println("\nReached maximum duration (1 hour)...")
				break loop
			}
		}

		// Evaluate fitness for each individual playlist (parallelized with worker pool)
		group := pool.Group()
		for i := range population {
			idx := i
			group.Submit(func() {
				scoredPopulation[idx] = Individual{
					Genes: population[idx],
					Score: calculateFitness(population[idx]),
				}
			})
		}
		group.Wait()

		// Sort population from lowest score (better fit) to highest (worse fit)
		slices.SortFunc(scoredPopulation, func(a Individual, b Individual) int {
			return int(a.Score - b.Score)
		})

		// Elitism-based immigration: Replace worst 5% with mutated elite individuals
		// This maintains diversity while keeping immigrants "semi-adapted" to the landscape
		immigrantCount := int(float64(populationSize) * immigrationRate)
		for i := 0; i < immigrantCount; i++ {
			worstIdx := len(scoredPopulation) - 1 - i
			// Clone the best individual and mutate it (10-20% swaps)
			scoredPopulation[worstIdx].Genes = slices.Clone(scoredPopulation[0].Genes)
			numSwaps := len(tracks) / 10 // About 10% of playlist size
			if numSwaps < 3 {
				numSwaps = 3
			}
			for s := 0; s < numSwaps; s++ {
				a := rand.Intn(len(scoredPopulation[worstIdx].Genes))
				b := rand.Intn(len(scoredPopulation[worstIdx].Genes))
				scoredPopulation[worstIdx].Genes[a], scoredPopulation[worstIdx].Genes[b] = scoredPopulation[worstIdx].Genes[b], scoredPopulation[worstIdx].Genes[a]
			}
			// Mark with high score so it doesn't interfere with elitism
			scoredPopulation[worstIdx].Score = math.MaxFloat64
		}

		var parents [][]playlist.Track

		// Keep top 2 as elite parents (indices 0 and 1) - they stay unchanged
		parents = append(parents, scoredPopulation[0].Genes)
		parents = append(parents, scoredPopulation[1].Genes)

		// Fill the rest (indices 2 onwards) with tournament selection
		for i := 2; i < len(scoredPopulation); i++ {
			// Tournament selection: pick best of N random individuals
			bestIdx := rand.Intn(len(scoredPopulation))
			bestScore := scoredPopulation[bestIdx].Score
			for j := 1; j < tournamentSize; j++ {
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

		// Keep top 2 performing playlist unchanged as children
		copy(children[0], parents[0])
		copy(children[1], parents[1])

		// Create offspring through crossover for the rest
		for i := 2; i < len(parents)-1; i += 2 {
			edgeRecombinationCrossover(children[i], parents[i], parents[i+1], crossoverEdges, parent1Index, parent2Index, seenEdges, usedTracks)
			edgeRecombinationCrossover(children[i+1], parents[i+1], parents[i], crossoverEdges, parent1Index, parent2Index, seenEdges, usedTracks)
		}

		// Handle odd population size (if any)
		if len(parents)%2 == 1 {
			edgeRecombinationCrossover(children[len(parents)-1], parents[len(parents)-1], parents[0], crossoverEdges, parent1Index, parent2Index, seenEdges, usedTracks)
		}

		// Apply 2-opt local search to elite children (polish the best solutions) - parallelized with worker pool
		topCount := int(float64(len(children)) * elitePercentage)
		if topCount < 2 {
			topCount = 2
		}
		group = pool.Group()
		for i := 0; i < topCount; i++ {
			idx := i
			group.Submit(func() {
				twoOptImprove(children[idx])
			})
		}
		group.Wait()

		// Calculate adaptive mutation rate based on convergence
		// Starts at 0.3 (high exploration), decays to 0.1 (fine-tuning) over 100 generations without improvement
		mutationRate := maxMutationRate - (float64(generationsWithoutImprovement)/mutationDecayGen)*(maxMutationRate-minMutationRate)
		if mutationRate < minMutationRate {
			mutationRate = minMutationRate
		}

		// Mutate offspring (skip top performing playlists at indices 0 and 1)
		for i := 2; i < len(children); i++ {
			if rand.Float64() < mutationRate {
				// 50% chance of swap vs inversion
				if rand.Float64() < 0.5 {
					// Swap mutation: swap individual tracks multiple times
					numSwaps := minSwapMutations + rand.Intn(maxSwapMutations-minSwapMutations+1)
					for s := 0; s < numSwaps; s++ {
						a := rand.Intn(len(children[i]))
						b := rand.Intn(len(children[i]))
						children[i][a], children[i][b] = children[i][b], children[i][a]
					}
				} else {
					// Inversion: reverse a substring to escape local minima
					start := rand.Intn(len(children[i]))
					end := rand.Intn(len(children[i]))
					if start > end {
						start, end = end, start
					}
					// Reverse children[i][start:end+1]
					for start < end {
						children[i][start], children[i][end] = children[i][end], children[i][start]
						start++
						end--
					}
				}
			}
		}

		// Swap populations (double buffering - reuse memory)
		population, nextPopulation = nextPopulation, population

		// Track best individual across all generations
		if scoredPopulation[0].Score < bestFitness {
			bestFitness = scoredPopulation[0].Score
			bestIndividual = slices.Clone(scoredPopulation[0].Genes)
			generationsWithoutImprovement = 0
		} else {
			generationsWithoutImprovement++
		}

		// Print progress when fitness improves
		fitnessImproved := scoredPopulation[0].Score < previousBestFitness
		enoughGensPassed := gen-lastPrintedGen >= 10

		if fitnessImproved && enoughGensPassed {
			// Clear status line before printing progress
			fmt.Print("\r\033[K")
			elapsed := time.Since(startTime)
			elapsedStr := formatElapsed(elapsed)
			fmt.Printf("%s Gen %d - fitness: %.2f\n", elapsedStr, gen, scoredPopulation[0].Score)
			previousBestFitness = scoredPopulation[0].Score
			lastPrintedGen = gen
		}
		gen++
	}

	// Clear status line at end
	fmt.Print("\r\033[K")

	fmt.Printf("\nCompleted %d generations in %v\n", gen, time.Since(startTime).Round(time.Millisecond))

	// Return best individual found across all generations (no re-evaluation needed)
	return bestIndividual
}

// calculateFitness computes the fitness score for a given playlist ordering
// This is a convenience wrapper around segmentFitness for the full playlist
func calculateFitness(individual []playlist.Track) float64 {
	return segmentFitness(individual, 0, len(individual)-1)
}

// edgeFitnessCache stores pre-calculated fitness values for track transitions
// Indexed by [fromTrackIdx][toTrackIdx] for O(1) lookup
var edgeFitnessCache [][]float64

// buildEdgeFitnessCache pre-calculates fitness for all possible track pairs
// This eliminates repeated calculations of harmonic distance, artist/album penalties,
// energy deltas, and BPM differences during the genetic algorithm
func buildEdgeFitnessCache(tracks []playlist.Track) {
	n := len(tracks)

	// Set Index field on each track for fast lookups
	for i := range tracks {
		tracks[i].Index = i
	}

	// Allocate 2D array for edge fitness
	edgeFitnessCache = make([][]float64, n)
	for i := range edgeFitnessCache {
		edgeFitnessCache[i] = make([]float64, n)
	}

	// Pre-calculate fitness for all track pairs
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i == j {
				continue // Skip self-edges
			}

			t1, t2 := &tracks[i], &tracks[j]
			fitness := 0.0

			// Harmonic distance
			distance := playlist.HarmonicDistanceParsed(t1.ParsedKey, t2.ParsedKey)
			fitness += float64(distance)

			// Artist penalty
			if t1.Artist == t2.Artist {
				fitness += sameArtistPenalty
			}

			// Album penalty
			if t1.Album == t2.Album {
				fitness += sameAlbumPenalty
			}

			// Energy delta
			fitness += math.Abs(float64(t1.Energy-t2.Energy)) * energyDeltaWeight

			// BPM delta
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
				fitness += minBPMDistance * bpmDeltaWeight
			}

			// Store in cache [i][j]
			edgeFitnessCache[i][j] = fitness
		}
	}
}

// segmentFitness calculates fitness contribution for a track segment
//
// Fitness is minimized (lower = better) and includes:
// - Cached edge fitness (harmonic distance, artist/album penalties, energy/BPM deltas)
// - Position bias: encourage low-energy tracks near the start of playlist
//
// Returns the total fitness score for tracks[start:end+1]
func segmentFitness(tracks []playlist.Track, start, end int) float64 {
	fitness := 0.0
	biasThreshold := int(float64(len(tracks)) * lowEnergyBiasPortion)

	// Calculate fitness for the segment [start:end+1]
	for j := start; j <= end; j++ {
		// Add cached edge fitness for transition from j-1 to j
		if j > 0 {
			fitness += edgeFitnessCache[tracks[j-1].Index][tracks[j].Index]
		}

		// Position-based energy penalty: bias first 20% of playlist towards low energy
		if j < biasThreshold {
			positionWeight := 1.0 - float64(j)/float64(biasThreshold) // 1.0 → 0.0
			energyPositionPenalty := float64(tracks[j].Energy) * positionWeight * lowEnergyBiasWeight
			fitness += energyPositionPenalty
		}
	}

	return fitness
}

// twoOptImprove applies 2-opt local search to polish a playlist
//
// 2-opt is a local search heuristic that:
// 1. Tests every possible segment reversal
// 2. Keeps reversals that improve fitness
// 3. Repeats until no improvements found
//
// Uses delta evaluation: only recalculates fitness for the affected
// segment rather than the entire playlist, making it much faster.
//
// Uses "don't look bits" optimization: tracks positions that recently
// failed to improve and skips them, significantly reducing redundant checks.
//
// This is applied to elite individuals (top 10%) each generation
// to intensify the search around good solutions.
func twoOptImprove(tracks []playlist.Track) {
	n := len(tracks)
	improved := true

	// Don't look bits: track positions that recently failed to improve
	dontLook := make([]bool, n)

	// Calculate initial full fitness once
	currentFitness := calculateFitness(tracks)

	// Keep iterating until no more improvements found
	for improved {
		improved = false

		// Try every possible pair of positions (i, j) where i < j
		for i := 0; i < n-2; i++ {
			if dontLook[i] {
				continue // Skip positions that recently failed to improve
			}

			positionImproved := false
			for j := i + 2; j < n; j++ {
				// Calculate old fitness contribution for affected region
				// Region: positions i through min(j+1, n-1) (inclusive of boundary edges)
				endPos := j + 1
				if endPos >= n {
					endPos = n - 1
				}
				oldSegmentFitness := segmentFitness(tracks, i, endPos)

				// Reverse segment [i+1 : j] (inclusive)
				reverseSegment(tracks, i+1, j)

				// Calculate new fitness contribution for the same region
				newSegmentFitness := segmentFitness(tracks, i, endPos)

				// Calculate delta and new total fitness
				delta := newSegmentFitness - oldSegmentFitness
				newFitness := currentFitness + delta

				// If improvement found, keep it and continue searching
				if newFitness < currentFitness {
					currentFitness = newFitness
					improved = true
					positionImproved = true
					// Reset all don't-look bits since the landscape changed
					for k := range dontLook {
						dontLook[k] = false
					}
					// Don't reverse back - we're keeping this improvement
				} else {
					// No improvement - reverse back to original
					reverseSegment(tracks, i+1, j)
				}
			}

			// If position i didn't improve across all j values, mark it
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
//
// Edge Recombination Crossover (ERC) is specifically designed for ordering problems:
// - Builds an edge table from both parents showing track adjacencies
// - Constructs offspring by preferring transitions that appear in parents
// - Greedily selects next track with fewest unused connections
//
// This preserves more of the parents' good features than simple crossover methods,
// making it ideal for playlist ordering where track adjacency matters.
//
// The buffers (edges, p1Index, etc.) are reused across calls for efficiency.
func edgeRecombinationCrossover(dst, parent1, parent2 []playlist.Track, edges [][]int, p1Index, p2Index map[string]int, seen []map[int]bool, used []bool) {
	numTracks := len(parent1)

	// Build edge table: create adjacency lists from both parents
	for i := 0; i < numTracks; i++ {
		p1Index[parent1[i].Path] = i
		p2Index[parent2[i].Path] = i
	}

	// Reset edges, clear seen maps, and add parent1 edges
	for i := 0; i < numTracks; i++ {
		edges[i] = edges[i][:0]
		for k := range seen[i] {
			delete(seen[i], k)
		}
		// Add parent1's immediate left/right neighbors
		if i > 0 {
			edges[i] = append(edges[i], i-1)
			seen[i][i-1] = true
		}
		if i < numTracks-1 {
			edges[i] = append(edges[i], i+1)
			seen[i][i+1] = true
		}
	}

	// Add edges from parent2 (neighbors in parent2's ordering)
	for i1, track1 := range parent1 {
		i2 := p2Index[track1.Path]

		// Add parent2's left neighbor
		if i2 > 0 {
			j1 := p1Index[parent2[i2-1].Path]
			if !seen[i1][j1] {
				edges[i1] = append(edges[i1], j1)
				seen[i1][j1] = true
			}
		}

		// Add parent2's right neighbor
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
		// Select best neighbor: pick unused neighbor with fewest remaining unused edges
		nextIdx := -1
		minEdges := math.MaxInt

		for _, neighbor := range edges[currentIdx] {
			if used[neighbor] {
				continue
			}

			// Count unused edges for this neighbor
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
				// Should never happen, but safety check
				break
			}
		}

		dst[pos] = parent1[nextIdx]
		used[nextIdx] = true
		currentIdx = nextIdx
	}
}
