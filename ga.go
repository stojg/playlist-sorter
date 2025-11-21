// ABOUTME: Core genetic algorithm implementation for playlist optimization
// ABOUTME: Includes fitness calculation, crossover, mutation, and 2-opt local search

package main

import (
	"cmp"
	"context"
	"math"
	"math/rand/v2"
	"runtime"
	"slices"
	"sync"
	"time"

	"playlist-sorter/config"
	"playlist-sorter/playlist"
)

// workerPool manages parallel task execution with submit-and-wait pattern
type workerPool struct {
	workers  int
	taskChan chan func()
	workerWg sync.WaitGroup // tracks worker lifetime
	taskWg   sync.WaitGroup // tracks task completion
}

// newWorkerPool creates worker pool sized to available CPUs
func newWorkerPool(bufferSize int) *workerPool {
	numWorkers := runtime.NumCPU()
	pool := &workerPool{
		workers:  numWorkers,
		taskChan: make(chan func(), bufferSize),
	}

	for range numWorkers {
		pool.workerWg.Add(1)

		go func() {
			defer pool.workerWg.Done()

			for task := range pool.taskChan {
				task()
				pool.taskWg.Done()
			}
		}()
	}

	return pool
}

// submit adds task to pool, blocks if channel full
func (p *workerPool) submit(task func()) {
	p.taskWg.Add(1)
	p.taskChan <- task
}

// wait blocks until all tasks complete
func (p *workerPool) wait() {
	p.taskWg.Wait()
}

// close shuts down pool and waits for workers to exit
func (p *workerPool) close() {
	close(p.taskChan)
	p.workerWg.Wait()
}

const (
	maxDuration = 1 * time.Hour

	populationSize        = 100
	immigrationRate       = 0.15
	immigrantSwapsDivisor = 10
	elitePercentage       = 0.03
	tournamentSize        = 3

	seedOriginalOrder = 0
	seedEnergySorted  = 1
	seedBPMSorted     = 2
	seedKeySorted     = 3
	seedRandomStart   = 4

	maxMutationRate  = 0.3
	minMutationRate  = 0.1
	mutationDecayGen = 100.0
	minSwapMutations = 2
	maxSwapMutations = 5

	twoOptStartGen       = 50
	twoOptIntervalGens   = 100
	floatingPointEpsilon = 1e-10

	updateIntervalGenerations = 50

	camelotWheelPositions = 12
)

// Individual represents a candidate solution (lower score = better)
type Individual struct {
	Genes []playlist.Track
	Score float64
}

// Compare returns -1 if better, 0 if equal, 1 if worse
func (ind Individual) Compare(other Individual) int {
	return cmp.Compare(ind.Score, other.Score)
}

// GAUpdate contains GA state information
type GAUpdate struct {
	Epoch        int
	Generation   int
	BestFitness  float64
	BestPlaylist []playlist.Track
	GenPerSec    float64
	Breakdown    playlist.Breakdown
}

// minBPMDistance finds minimum BPM difference considering half/double time mixing
func minBPMDistance(bpm1, bpm2 float64) float64 {
	distances := []float64{
		math.Abs(bpm1 - bpm2),
		math.Abs(bpm1*0.5 - bpm2),
		math.Abs(bpm1 - bpm2*0.5),
		math.Abs(bpm1*2.0 - bpm2),
		math.Abs(bpm1 - bpm2*2.0),
	}

	minDist := distances[0]
	for _, d := range distances[1:] {
		if d < minDist {
			minDist = d
		}
	}

	return minDist
}

// EdgeData stores pre-calculated values for track transitions (weights applied at eval time)
type EdgeData struct {
	HarmonicDistance int
	SameArtist       bool
	SameAlbum        bool
	EnergyDelta      float64
	BPMDelta         float64
	GenreDifference  float64 // 0.0 = same, 1.0 = different
}

// FitnessNormalizers stores max values for normalizing components to [0,1]
type FitnessNormalizers struct {
	MaxHarmonic     float64
	MaxSameArtist   float64
	MaxSameAlbum    float64
	MaxEnergyDelta  float64
	MaxBPMDelta     float64
	MaxPositionBias float64
	MaxGenreChange  float64
}

// GAContext holds pre-calculated data for fitness evaluation
type GAContext struct {
	edgeCache   [][]EdgeData
	normalizers FitnessNormalizers
}

// geneticSort optimizes track ordering using GA with fitness-based selection, crossover, mutation,
// and 2-opt local search. Runs until context cancelled or 1 hour timeout.
func geneticSort(ctx context.Context, tracks []playlist.Track, sharedConfig *config.SharedConfig, updateChan chan<- GAUpdate, epoch int, gaCtx *GAContext) []playlist.Track {
	var (
		startTime    = time.Now()
		gen          = 0
		genesLen     = len(tracks)
		lastGenTime  = time.Now()
		lastGenCount = 0
	)

	config := sharedConfig.Get()

	workerPool := newWorkerPool(runtime.NumCPU())
	defer workerPool.close()

	scoredPopulation := make([]Individual, populationSize)
	for i := range scoredPopulation {
		scoredPopulation[i].Genes = make([]playlist.Track, genesLen)
	}

	presentMap := make(map[string]bool, genesLen)

	nextGen := make([][]playlist.Track, populationSize)
	for i := range populationSize {
		nextGen[i] = make([]playlist.Track, genesLen)
	}

	currentGen := make([][]playlist.Track, populationSize)

	currentGen[seedOriginalOrder] = slices.Clone(tracks)

	currentGen[seedEnergySorted] = slices.Clone(tracks)
	slices.SortFunc(currentGen[seedEnergySorted], func(a, b playlist.Track) int { return a.Energy - b.Energy })

	currentGen[seedBPMSorted] = slices.Clone(tracks)
	slices.SortFunc(currentGen[seedBPMSorted], func(a, b playlist.Track) int { return cmp.Compare(a.BPM, b.BPM) })

	currentGen[seedKeySorted] = slices.Clone(tracks)
	slices.SortFunc(currentGen[seedKeySorted], func(a, b playlist.Track) int { return a.ParsedKey.Compare(b.ParsedKey) })

	for i := seedRandomStart; i < populationSize; i++ {
		currentGen[i] = slices.Clone(tracks)
		rand.Shuffle(len(currentGen[i]), func(a, b int) { currentGen[i][a], currentGen[i][b] = currentGen[i][b], currentGen[i][a] })
	}

	var (
		bestIndividual                []playlist.Track
		bestFitness                   = math.MaxFloat64
		generationsWithoutImprovement = 0
	)

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		default:
			if time.Since(startTime) >= maxDuration {
				break loop
			}
		}

		debugf("[GA] Getting config for gen %d", gen)
		config = sharedConfig.Get()
		debugf("[GA] Config retrieved - Genre Weight: %.2f", config.GenreWeight)

		debugf("[GA] Starting fitness evaluation for gen %d", gen)
		for i := range currentGen {
			workerPool.submit(func() {
				scoredPopulation[i] = Individual{Genes: currentGen[i], Score: calculateFitness(currentGen[i], config, gaCtx)}
			})
		}
		workerPool.wait()
		debugf("[GA] Fitness evaluation complete for gen %d", gen)

		slices.SortFunc(scoredPopulation, func(a, b Individual) int { return a.Compare(b) })

		shouldRunTwoOpt := gen >= twoOptStartGen && (gen == twoOptStartGen || (gen-twoOptStartGen)%twoOptIntervalGens == 0)
		if shouldRunTwoOpt {
			topCount := int(float64(populationSize) * elitePercentage)
			if topCount < 2 {
				topCount = 2
			}
			debugf("[GA] Starting 2-opt for gen %d (topCount=%d)", gen, topCount)
			for i := range topCount {
				workerPool.submit(func() {
					twoOptImprove(scoredPopulation[i].Genes, config, gaCtx)
				})
			}
			workerPool.wait()
			debugf("[GA] 2-opt complete for gen %d", gen)
		}

		fitnessImproved := false
		if scoredPopulation[0].Score < bestFitness {
			bestFitness = scoredPopulation[0].Score
			bestIndividual = slices.Clone(scoredPopulation[0].Genes)
			generationsWithoutImprovement = 0
			fitnessImproved = true
		} else {
			generationsWithoutImprovement++
		}

		if updateChan != nil && (fitnessImproved || gen%updateIntervalGenerations == 0) {
			now := time.Now()
			elapsed := now.Sub(lastGenTime).Seconds()
			genPerSec := 0.0
			if elapsed > 0 {
				genPerSec = float64(gen-lastGenCount) / elapsed
			}

			config = sharedConfig.Get()
			breakdown := calculateFitnessWithBreakdown(bestIndividual, config, gaCtx)

			select {
			case updateChan <- GAUpdate{
				Epoch:        epoch,
				Generation:   gen,
				BestFitness:  breakdown.Total,
				BestPlaylist: slices.Clone(bestIndividual),
				GenPerSec:    genPerSec,
				Breakdown:    breakdown,
			}:
			default:
			}

			lastGenTime = now
			lastGenCount = gen
		}

		immigrantCount := int(float64(populationSize) * immigrationRate)
		immigrantSwaps := genesLen / immigrantSwapsDivisor
		if immigrantSwaps < 3 {
			immigrantSwaps = 3
		}

		for i := range immigrantCount {
			worstIdx := len(scoredPopulation) - 1 - i
			copy(scoredPopulation[worstIdx].Genes, scoredPopulation[0].Genes)
			for range immigrantSwaps {
				a := rand.IntN(genesLen)
				b := rand.IntN(genesLen)
				scoredPopulation[worstIdx].Genes[a], scoredPopulation[worstIdx].Genes[b] = scoredPopulation[worstIdx].Genes[b], scoredPopulation[worstIdx].Genes[a]
			}
			scoredPopulation[worstIdx].Score = calculateFitness(scoredPopulation[worstIdx].Genes, config, gaCtx)
		}

		parents := make([][]playlist.Track, populationSize)

		parents[0] = scoredPopulation[0].Genes
		parents[1] = scoredPopulation[1].Genes

		for i := 2; i < len(scoredPopulation); i++ {
			bestIdx := rand.IntN(len(scoredPopulation))
			bestScore := scoredPopulation[bestIdx].Score
			for j := 1; j < tournamentSize; j++ {
				idx := rand.IntN(len(scoredPopulation))
				if scoredPopulation[idx].Score < bestScore {
					bestIdx = idx
					bestScore = scoredPopulation[idx].Score
				}
			}
			parents[i] = scoredPopulation[bestIdx].Genes
		}

		copy(nextGen[0], scoredPopulation[0].Genes)
		copy(nextGen[1], scoredPopulation[1].Genes)

		for i := 2; i < len(parents)-1; i += 2 {
			orderCrossover(nextGen[i], parents[i], parents[i+1], presentMap)
			orderCrossover(nextGen[i+1], parents[i+1], parents[i], presentMap)
		}
		if len(parents)%2 == 1 {
			orderCrossover(nextGen[len(parents)-1], parents[len(parents)-1], parents[0], presentMap)
		}

		mutationRate := minMutationRate + (float64(generationsWithoutImprovement)/mutationDecayGen)*(maxMutationRate-minMutationRate)
		if mutationRate > maxMutationRate {
			mutationRate = maxMutationRate
		}

		for i := 2; i < populationSize; i++ {
			if rand.Float64() < mutationRate {
				if rand.Uint32()&1 == 0 {
					numSwaps := minSwapMutations + rand.IntN(maxSwapMutations-minSwapMutations+1)
					for range numSwaps {
						a := rand.IntN(genesLen)
						b := rand.IntN(genesLen)
						nextGen[i][a], nextGen[i][b] = nextGen[i][b], nextGen[i][a]
					}
				} else {
					start := rand.IntN(genesLen)
					end := rand.IntN(genesLen)
					if start > end {
						start, end = end, start
					}
					reverseSegment(nextGen[i], start, end)
				}
			}
		}

		currentGen, nextGen = nextGen, currentGen

		debugf("[GA] Generation %d complete", gen)
		gen++
	}

	return bestIndividual
}

// buildEdgeFitnessCache pre-calculates base values for track pairs (weights applied at eval time)
func buildEdgeFitnessCache(tracks []playlist.Track) *GAContext {
	n := len(tracks)

	ctx := &GAContext{
		edgeCache: make([][]EdgeData, n),
	}

	for i := range ctx.edgeCache {
		ctx.edgeCache[i] = make([]EdgeData, n)
	}

	for i := range n {
		for j := range n {
			if i == j {
				continue
			}

			t1, t2 := &tracks[i], &tracks[j]

			harmonicDist := playlist.HarmonicDistanceParsed(t1.ParsedKey, t2.ParsedKey)

			sameArtist := t1.Artist == t2.Artist
			sameAlbum := t1.Album == t2.Album

			energyDelta := math.Abs(float64(t1.Energy - t2.Energy))

			bpmDelta := 0.0
			if t1.BPM > 0 && t2.BPM > 0 {
				bpmDelta = minBPMDistance(t1.BPM, t2.BPM)
			}

			genreDiff := playlist.GenreSimilarity(t1.Genre, t2.Genre)

			ctx.edgeCache[i][j] = EdgeData{
				HarmonicDistance: harmonicDist,
				SameArtist:       sameArtist,
				SameAlbum:        sameAlbum,
				EnergyDelta:      energyDelta,
				BPMDelta:         bpmDelta,
				GenreDifference:  genreDiff,
			}
		}
	}

	ctx.normalizers.MaxHarmonic = camelotWheelPositions * float64(n-1)

	ctx.normalizers.MaxSameArtist = float64(n - 1)
	ctx.normalizers.MaxSameAlbum = float64(n - 1)

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

	ctx.normalizers.MaxEnergyDelta = (maxEnergy - minEnergy) * float64(n-1)

	maxBPMDist := 0.0

	for i := range n {
		for j := range n {
			if i != j && tracks[i].BPM > 0 && tracks[j].BPM > 0 {
				if ctx.edgeCache[i][j].BPMDelta > maxBPMDist {
					maxBPMDist = ctx.edgeCache[i][j].BPMDelta
				}
			}
		}
	}

	ctx.normalizers.MaxBPMDelta = maxBPMDist * float64(n-1)

	ctx.normalizers.MaxGenreChange = float64(n - 1)

	ctx.normalizers.MaxPositionBias = maxEnergy

	return ctx
}

// calculateFitness computes the fitness score for a given playlist ordering
func calculateFitness(individual []playlist.Track, config config.GAConfig, ctx *GAContext) float64 {
	breakdown := calculateFitnessWithBreakdown(individual, config, ctx)

	return breakdown.Total
}

// calculateFitnessWithBreakdown computes fitness and returns detailed breakdown
func calculateFitnessWithBreakdown(individual []playlist.Track, config config.GAConfig, ctx *GAContext) playlist.Breakdown {
	return segmentFitnessWithBreakdown(individual, 0, len(individual)-1, config, ctx)
}

// orderCrossover (OX) creates offspring by preserving order from parents.
// Copies random substring from parent1, fills rest from parent2 in order.
func orderCrossover(dst, parent1, parent2 []playlist.Track, present map[string]bool) {
	numTracks := len(parent1)

	clear(present)

	cut1 := rand.IntN(numTracks)
	cut2 := rand.IntN(numTracks)

	if cut1 > cut2 {
		cut1, cut2 = cut2, cut1
	}

	for i := cut1; i <= cut2; i++ {
		dst[i] = parent1[i]
		present[parent1[i].Path] = true
	}

	dstIdx := (cut2 + 1) % numTracks

	for i := range numTracks {
		parent2Idx := (cut2 + 1 + i) % numTracks
		if !present[parent2[parent2Idx].Path] {
			dst[dstIdx] = parent2[parent2Idx]
			dstIdx = (dstIdx + 1) % numTracks
		}
	}
}

// twoOptImprove applies 2-opt local search by systematically testing segment reversals.
// Uses delta evaluation (only recalc changed segment), don't-look-bits optimization,
// and epsilon threshold to prevent floating point oscillation.
func twoOptImprove(tracks []playlist.Track, config config.GAConfig, ctx *GAContext) {
	n := len(tracks)

	positionsExhausted := make([]bool, n)

	currentFitness := calculateFitness(tracks, config, ctx)

	const maxIterations = 1000

	iteration := 0

	improved := true
	for improved && iteration < maxIterations {
		improved = false
		iteration++

		for i := range n - 1 {
			if positionsExhausted[i] {
				continue
			}

			positionImproved := false

			for j := i + 1; j < n; j++ {
				endPos := j + 1
				if endPos >= n {
					endPos = n - 1
				}

				oldSegmentFitness := segmentFitness(tracks, i, endPos, config, ctx)

				reverseSegment(tracks, i, j)
				newSegmentFitness := segmentFitness(tracks, i, endPos, config, ctx)

				newFitness := currentFitness + newSegmentFitness - oldSegmentFitness

				if !hasFitnessImproved(newFitness, currentFitness, floatingPointEpsilon) {
					reverseSegment(tracks, i, j)

					continue
				}

				currentFitness = newFitness
				improved = true
				positionImproved = true

				clear(positionsExhausted)
			}

			if !positionImproved {
				positionsExhausted[i] = true
			}
		}
	}

	if iteration >= maxIterations {
		debugf("[2-OPT] Hit max iterations (%d)", maxIterations)
	}
}

// segmentFitness calculates fitness for track segment
func segmentFitness(tracks []playlist.Track, start, end int, config config.GAConfig, ctx *GAContext) float64 {
	return segmentFitnessWithBreakdown(tracks, start, end, config, ctx).Total
}

// segmentFitnessWithBreakdown calculates fitness with component breakdown
func segmentFitnessWithBreakdown(tracks []playlist.Track, start, end int, config config.GAConfig, ctx *GAContext) playlist.Breakdown {
	var breakdown playlist.Breakdown

	biasThreshold := int(float64(len(tracks)) * config.LowEnergyBiasPortion)
	genreEnabled := config.GenreWeight != 0 && ctx.normalizers.MaxGenreChange > 0

	var genreAbsWeight, genreSign float64

	if genreEnabled {
		genreAbsWeight = math.Abs(config.GenreWeight) / ctx.normalizers.MaxGenreChange

		if config.GenreWeight > 0 {
			genreSign = 1.0
		} else {
			genreSign = -1.0
		}
	}

	for j := start; j <= end; j++ {
		if j > 0 { //nolint:nestif
			idx1 := tracks[j-1].Index
			idx2 := tracks[j].Index
			edge := ctx.edgeCache[idx1][idx2]

			breakdown.Harmonic += applyWeightedPenalty(float64(edge.HarmonicDistance), ctx.normalizers.MaxHarmonic, config.HarmonicWeight)

			if edge.SameArtist {
				breakdown.SameArtist += applyWeightedPenalty(1.0, ctx.normalizers.MaxSameArtist, config.SameArtistPenalty)
			}

			if edge.SameAlbum {
				breakdown.SameAlbum += applyWeightedPenalty(1.0, ctx.normalizers.MaxSameAlbum, config.SameAlbumPenalty)
			}

			breakdown.EnergyDelta += applyWeightedPenalty(edge.EnergyDelta, ctx.normalizers.MaxEnergyDelta, config.EnergyDeltaWeight)

			breakdown.BPMDelta += applyWeightedPenalty(edge.BPMDelta, ctx.normalizers.MaxBPMDelta, config.BPMDeltaWeight)

			if genreEnabled {
				rawPenalty := edge.GenreDifference
				if genreSign < 0 {
					rawPenalty = 1.0 - rawPenalty
				}

				breakdown.GenreChange += rawPenalty * genreAbsWeight
			}
		}

		if j < biasThreshold {
			positionWeight := 1.0 - float64(j)/float64(biasThreshold)
			rawPositionBias := float64(tracks[j].Energy) * positionWeight
			normalizedPositionBias := rawPositionBias / ctx.normalizers.MaxPositionBias
			energyPositionPenalty := normalizedPositionBias * config.LowEnergyBiasWeight
			breakdown.PositionBias += energyPositionPenalty
		}
	}

	breakdown.Total = breakdown.Harmonic + breakdown.SameArtist + breakdown.SameAlbum +
		breakdown.EnergyDelta + breakdown.BPMDelta + breakdown.PositionBias + breakdown.GenreChange

	return breakdown
}

// applyWeightedPenalty normalizes value to [0,1] and applies weight
func applyWeightedPenalty(rawValue, maxValue, weight float64) float64 {
	normalized := rawValue / maxValue

	return normalized * weight
}

// reverseSegment reverses tracks[start:end+1] in place
func reverseSegment(tracks []playlist.Track, start, end int) {
	for start < end {
		tracks[start], tracks[end] = tracks[end], tracks[start]
		start++
		end--
	}
}

// calculateTheoreticalMinimum calculates theoretical minimum fitness (not achievable due to conflicting constraints)
func calculateTheoreticalMinimum(tracks []playlist.Track, config config.GAConfig, ctx *GAContext) float64 {
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

	if ctx.normalizers.MaxEnergyDelta > 0 {
		minEnergyDelta = (minEnergyDelta / ctx.normalizers.MaxEnergyDelta) * config.EnergyDeltaWeight
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

	if ctx.normalizers.MaxBPMDelta > 0 && len(bpms) > 1 {
		minBPMDelta = (minBPMDelta / ctx.normalizers.MaxBPMDelta) * config.BPMDeltaWeight
	}

	// 6. Position Bias: Best case = lowest energy tracks at start
	biasThreshold := int(float64(n) * config.LowEnergyBiasPortion)
	minPositionBias := 0.0

	for j := 0; j < biasThreshold && j < n; j++ {
		positionWeight := 1.0 - float64(j)/float64(biasThreshold)

		rawBias := float64(energies[j]) * positionWeight
		if ctx.normalizers.MaxPositionBias > 0 {
			minPositionBias += (rawBias / ctx.normalizers.MaxPositionBias) * config.LowEnergyBiasWeight
		}
	}

	// 7. Genre: Best case = 0 (either all same genre or all different, depending on weight direction)
	minGenre := 0.0

	return minHarmonic + minSameArtist + minSameAlbum +
		minEnergyDelta + minBPMDelta + minPositionBias + minGenre
}
