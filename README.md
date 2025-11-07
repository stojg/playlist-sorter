# Playlist Sorter

A high-performance genetic algorithm-based playlist optimizer that sorts M3U8 playlists using harmonic mixing principles, energy flow optimization, and BPM compatibility analysis.

## Overview

This tool optimizes music playlists by minimizing:
- **Harmonic distance** between adjacent tracks using the Camelot wheel
- **Artist/album repetition** with configurable penalties
- **Energy level jumps** for smooth listening flow
- **BPM mismatches** accounting for half/double time mixing (common in electronic music)
- **Position bias** favoring low-energy tracks at the start of playlists

Uses a genetic algorithm with Edge Recombination Crossover (ERC) to preserve good track transitions, combined with 2-opt local search for polishing elite solutions.

## Features

- ✅ **Genetic Algorithm** with tournament selection, elitism, and immigration
- ✅ **Edge Recombination Crossover (ERC)** - preserves adjacency from parent solutions
- ✅ **2-opt Local Search** - polish top solutions with delta evaluation
- ✅ **Performance Optimized** - 2.33x speedup through profiling and optimization
- ✅ **Metadata Extraction** - reads BPM, key, energy, and artist/album from audio files
- ✅ **Camelot Wheel** - harmonic mixing compatibility
- ✅ **Configurable Parameters** - tune fitness weights for different genres

## Installation

### Prerequisites

- Go 1.24+ (uses new `tool` directive)
- Music files with metadata (ID3 tags)
- Audio files accessible at `/Volumes/music/Music/` (or modify path in code)

### Build

```bash
cd /Volumes/music/playlist-sorter
go build -o playlist-sorter
```

## Usage

### Basic Usage

```bash
# Sort a playlist
./playlist-sorter path/to/playlist.m3u8

# The playlist file will be overwritten with optimized ordering
# Press Ctrl+C to stop early (returns best solution found so far)
```

### With Profiling

```bash
# CPU profiling
./playlist-sorter -cpuprofile=cpu.prof playlist.m3u8
go tool pprof cpu.prof

# Memory profiling
./playlist-sorter -memprofile=mem.prof playlist.m3u8
go tool pprof mem.prof
```

### Expected Output

```
Reading playlist: morning_chill_dnb.m3u8
Loading metadata for 52 tracks...
[+] Processed 10/52 tracks...
[+] Processed 20/52 tracks...
[+] Processed 30/52 tracks...
[+] Processed 40/52 tracks...
[+] Processed 50/52 tracks...
[_] Loaded metadata for 52 tracks

Optimizing playlist... (press Ctrl+C to stop early, or wait up to 1 hour)
0s Gen 0 - fitness: 336.75
1s Gen 45 - fitness: 298.42
3s Gen 123 - fitness: 267.89
...
```

## Project Structure

```
playlist-sorter/
├── main.go                   # GA implementation and entry point
├── playlist/
│   ├── track.go             # Track metadata extraction from audio files
│   ├── playlist.go          # M3U8 read/write functions
│   └── harmonic.go          # Camelot wheel harmonic mixing utilities
├── go.mod                    # Go module with tool dependencies
├── .golangci.yml            # Linter configuration
├── CLAUDE.md                # Developer documentation
└── README.md                # This file
```

## Algorithm Details

### Genetic Algorithm

**Population**: 100 individuals
**Selection**: Tournament selection (size 3) with top 2 elitism
**Crossover**: Edge Recombination Crossover (ERC)
**Mutation**: 20% rate - 50/50 split between swap (2-5 tracks) and inversion
**Immigration**: 5% random individuals per generation for diversity
**Local Search**: 2-opt improvement on top 10% of population
**Timeout**: 1 hour maximum (or Ctrl+C)

### Fitness Function

Minimizes (lower = better):

```go
fitness = Σ(harmonic_distance)
        + same_artist_penalty * 5.0
        + same_album_penalty * 2.0
        + energy_delta * 3.0
        + bpm_delta * 0.25
        + position_bias_penalty
```

**Position Bias**: First 20% of playlist biased toward low-energy tracks (weight 10.0, linear decay)

**BPM Matching**: Considers half-time and double-time (0.5x, 1x, 2x multipliers)

### Harmonic Distance (Camelot Wheel)

- **0** = Same key (perfect match)
- **1** = ±1 semitone with same mode OR relative major/minor (excellent)
- **3** = ±1 semitone with different mode (acceptable)
- **Higher** = Less compatible

Compatible transitions: `8A → {8A, 8B, 7A, 9A, 7B, 9B}`

## Performance

After optimization through profiling:
- **54+ generations/second** on 52-track playlist
- **2.33x speedup** vs initial implementation
- Key optimizations:
  - Delta evaluation for 2-opt (only recalculates changed segments)
  - Inline comparisons instead of `math.Min()` in hot paths
  - Pre-parsed Camelot keys (parse once, lookup many times)
  - Double-buffered population arrays (minimize allocations)

## Development

### Code Quality Tools

This project uses Go 1.24+'s `tool` directive for development tools:

```bash
# Lint the code
go tool golangci-lint run

# Check for security vulnerabilities
go tool govulncheck ./...

# Run tests with better output
go tool gotestsum --format testname

# List all available tools
go tool
```

### Adding Development Tools

```bash
# Add a tool
go get -tool github.com/example/tool/cmd/tool@latest

# Run the tool
go tool tool [args]
```

### Pre-commit Checklist

1. Format: `go fmt ./...`
2. Lint: `go tool golangci-lint run`
3. Vulnerabilities: `go tool govulncheck ./...`
4. Build: `go build`
5. Test: `go test ./...` (when tests exist)

### Profiling Workflow

1. Run with profiling: `./playlist-sorter -cpuprofile=cpu.prof playlist.m3u8`
2. Analyze: `go tool pprof -http=:8080 cpu.prof`
3. **Focus on percentages, not absolute time** (system load varies)
4. Look for functions taking >10% of time

## Configuration

### Tuning Parameters

Edit constants in `main.go` to tune for your use case:

```go
// Genetic algorithm parameters
populationSize       = 100    // Number of candidates per generation
mutationRate         = 0.2    // 20% mutation rate (research-backed optimal)
immigrationRate      = 0.05   // 5% random immigration per generation
lowEnergyBiasPortion = 0.2    // Bias first 20% of playlist
lowEnergyBiasWeight  = 10.0   // Weight for energy position penalty

// Fitness penalty weights
sameArtistPenalty = 5.0   // Penalty for consecutive same artist
sameAlbumPenalty  = 2.0   // Penalty for consecutive same album
energyDeltaWeight = 3.0   // Weight for energy changes
bpmDeltaWeight    = 0.25  // Weight for BPM differences (lower = less strict)

// Selection parameters
tournamentSize   = 3    // Tournament selection pool size
elitePercentage  = 0.1  // Top 10% get 2-opt local search
```

**Genre-specific tuning**:
- **Drum & Bass**: Current settings work well (BPM weight 0.25, energy weight 3.0)
- **House/Techno**: Increase `bpmDeltaWeight` to 0.5 for stricter tempo matching
- **Ambient**: Decrease `energyDeltaWeight` to 1.0 for more variety
- **Hip Hop**: Increase `sameArtistPenalty` to 10.0 to avoid artist clustering

### Metadata Format

Tracks must have metadata embedded in audio files:

**Required Tags**:
- Artist (standard ID3 tag)
- Album (standard ID3 tag)
- Title (standard ID3 tag)

**Optional Tags**:
- BPM (custom tag: `BPM`, `TBPM`, `bpm`, or `tempo`)
- Comments field with format: `"8A - Energy 6"` (Camelot key + energy level)

## API Reference

### Track Struct

```go
type Track struct {
    Path      string      // Relative path in playlist
    Key       string      // Camelot key (e.g., "8A")
    ParsedKey *CamelotKey // Pre-parsed for fast lookups
    Artist    string
    Album     string
    Title     string
    Energy    int         // 1-10 (0 if not available)
    BPM       float64     // Beats per minute (0 if not available)
}
```

### Playlist Functions

```go
// Load playlist with metadata from audio files
tracks, err := playlist.LoadPlaylistWithMetadata("/path/to/playlist.m3u8")

// Write sorted tracks back to playlist
err := playlist.WritePlaylist("/path/to/playlist.m3u8", sortedTracks)
```

### Harmonic Functions

```go
// Calculate harmonic distance between keys (0 = best)
distance := playlist.HarmonicDistance("8A", "9A")  // Returns: 1

// Check compatibility (distance <= 2)
compatible := playlist.IsCompatible("8A", "7B")    // Returns: true

// Get all compatible keys for mixing
keys := playlist.GetCompatibleKeys("8A")
// Returns: ["8A", "8B", "7A", "9A", "7B", "9B"]

// Parse key for fast lookups
key, err := playlist.ParseCamelotKey("8A")

// Fast distance calculation with pre-parsed keys
distance := playlist.HarmonicDistanceParsed(key1, key2)
```

## Known Issues

- **G304 gosec warnings**: False positives for file I/O operations (expected for CLI tool)
- **No unit tests**: Future work - see `CLAUDE.md` for testing strategy

## Future Work

- [ ] Add unit tests for core algorithms
- [ ] Parallel fitness evaluation for large playlists
- [ ] Genre-specific BPM mixing rules
- [ ] Support for additional metadata sources (MusicBrainz API, etc.)
- [ ] Web UI for visualization and manual tweaking
- [ ] A/B testing framework for algorithm improvements

## Contributing

See `CLAUDE.md` for detailed development documentation including:
- Architecture deep dive
- Performance optimization notes
- Code quality standards
- Profiling best practices
- Go 1.24+ tool directive usage

## License

This project is part of a personal music library management system.

## Related

- [beets](https://beets.io/) - Music library manager
- [Camelot Wheel](https://mixedinkey.com/harmonic-mixing-guide/) - Harmonic mixing system
- [Mixed In Key](https://mixedinkey.com/) - Commercial DJ key detection software
