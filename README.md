# Playlist Sorter

A genetic algorithm-based playlist optimizer that sorts M3U8 playlists using harmonic mixing, energy flow, and BPM compatibility.

## Overview

Optimizes music playlists by minimizing:
- Harmonic distance between adjacent tracks (Camelot wheel)
- Artist/album repetition with configurable penalties
- Energy level jumps for smooth listening flow
- BPM mismatches accounting for half/double time mixing
- Position bias favoring low-energy tracks at the start

Uses a genetic algorithm with Order Crossover (OX) for exploration combined with 2-opt local search for exploitation.

## Features

- Genetic algorithm with tournament selection, elitism, and immigration
- Order Crossover (OX) for preserving relative ordering from parents
- 2-opt local search with delta evaluation for polishing elite solutions
- Interactive TUI mode with live parameter tuning
- Metadata extraction from audio files (ID3 tags via beets database)
- Normalized fitness components for equal weight influence
- Configurable genre clustering/spreading (signed weight)

## Installation

### Prerequisites

- Go 1.25+ (uses `math/rand/v2`)
- Music files with ID3 metadata (Artist, Album, Title, BPM)
- Comment field formatted as: `"8A - Energy 6"` (Camelot key and energy level)

### Build

```bash
# Production build (optimized with PGO)
make all

# Development build (with race detector and debug info)
make dev
```

## Usage

### Basic Usage

```bash
# Sort a playlist (overwrites file with optimized ordering)
./playlist-sorter path/to/playlist.m3u8

# Press Ctrl+C to stop early and use best solution found
```

### Interactive Mode

```bash
# Live parameter tuning with visual feedback
./playlist-sorter --visual path/to/playlist.m3u8
```

### View Mode

```bash
# Watch optimization progress in real-time (read-only)
./playlist-sorter --view path/to/playlist.m3u8
```

### Profiling

```bash
# CPU profiling
./playlist-sorter -cpuprofile=cpu.prof playlist.m3u8
go tool pprof -http=:8080 cpu.prof

# Memory profiling
./playlist-sorter -memprofile=mem.prof playlist.m3u8
go tool pprof mem.prof
```

## Project Structure

```
playlist-sorter/
├── main.go                   # CLI entry point and view mode
├── ga.go                     # Genetic algorithm core
├── config.go                 # Configuration management
├── tui.go                    # Interactive TUI mode
├── view.go                   # Read-only view mode
├── playlist/
│   ├── track.go             # Track metadata extraction
│   ├── playlist.go          # M3U8 read/write
│   └── harmonic.go          # Camelot wheel utilities
├── go.mod                    # Module with tool dependencies
├── .golangci.yml            # Linter configuration
└── README.md                # This file
```

## Algorithm

### Genetic Algorithm

- Population size: 100
- Selection: Tournament selection (size 3) with top 2 elitism
- Crossover: Order Crossover (OX)
- Mutation: Adaptive rate (10-30%), 50/50 swap/inversion
- Immigration: 15% per generation (mutated copies of best)
- Local search: 2-opt on top 3% starting at generation 50, then every 100 generations
- Timeout: 15 minutes maximum

### Fitness Function

All components normalized to [0,1] scale before applying weights for equal influence:

```
fitness = harmonic_distance
        + same_artist_penalty
        + same_album_penalty
        + energy_delta
        + bpm_delta
        + position_bias
        + genre_change  (optional, signed)
```

Default weights (configurable via TUI or config file):
- Harmonic: 0.5
- Energy delta: 0.5
- BPM delta: 0.5
- Same artist: 0.5
- Same album: 0.5
- Genre: 0.0 (disabled by default, -1.0 = spread, +1.0 = cluster)
- Position bias: 0.0 (disabled by default)

### Harmonic Distance (Camelot Wheel)

Based on Camelot wheel mixing principles:
- 0 = Same key (perfect)
- 1 = Adjacent key or relative major/minor (excellent)
- 2 = Parallel major/minor (dramatic shift, advanced)
- 10 = Undocumented transition (not a recognized mixing technique)

### BPM Matching

Considers half-time and double-time mixing (0.5x, 1x, 2x multipliers) to find minimum BPM distance.

## Performance

Optimizations through profiling (52-track playlist):
- 2,200+ generations in 5 seconds
- Key optimizations:
  - `math/rand/v2` for 12% sequential speedup
  - `Uint32()&1` for 23% faster coin flips
  - Delta evaluation for 2-opt (only recalculates changed segments)
  - Pre-parsed Camelot keys (parse once, lookup many times)
  - Generation buffer swapping (minimize allocations)

## Configuration

Config stored in `~/.config/playlist-sorter/config.json`. Edit via TUI (--visual) or manually.

### Metadata Requirements

Tracks must have:
- Artist, Album, Title (standard ID3 tags)
- BPM (custom tag: `BPM`, `TBPM`, `bpm`, or `tempo`)
- Comments field: `"8A - Energy 6"` (Camelot key + energy level 1-10)

## Development

### Build Modes

**Development** (with race detector and debug symbols):
```bash
make dev
./playlist-sorter-dev playlist.m3u8
```

**Production** (PGO-optimized and stripped):
```bash
make all  # Collects PGO profile, builds, and installs
```

### Race Detector

Always use race detector during development to catch concurrency bugs:
```bash
# Development builds include -race by default
make dev

# Or manually
go build -race -o playlist-sorter-dev

# Run with race detection
./playlist-sorter-dev playlist.m3u8
```

### Code Quality Tools

Using Go 1.25+ tool directive:

```bash
# Lint
make lint
# or: go tool golangci-lint run

# Format
go fmt ./...

# Security check
go tool govulncheck ./...
```

### Pre-commit

1. Format: `go fmt ./...`
2. Lint: `make lint`
3. Build: `make dev`
4. Test: Run on a playlist with race detector

### Profiling

Focus on percentages (>10% of time), not absolute values. System load affects absolute timing.

## API

### Track

```go
type Track struct {
    Path      string      // Relative path
    Key       string      // Camelot key (e.g., "8A")
    ParsedKey *CamelotKey // Pre-parsed for O(1) lookups
    Artist    string
    Album     string
    Title     string
    Genre     string
    Energy    int         // 1-10 (0 if unavailable)
    BPM       float64     // 0 if unavailable
    Index     int         // Original position
}
```

### Functions

```go
// Load playlist with metadata from beets
tracks, err := playlist.LoadPlaylistWithMetadata(path, verbose)

// Write sorted tracks back
err := playlist.WritePlaylist(path, tracks)

// Harmonic distance (0 = best)
distance := playlist.HarmonicDistance("8A", "9A")  // 1

// Pre-parsed for speed
key, _ := playlist.ParseCamelotKey("8A")
distance := playlist.HarmonicDistanceParsed(key1, key2)
```

## Known Issues

- G304 gosec warnings: False positives (CLI tool requires file I/O)
- No unit tests yet

## Future Work

- Unit tests for core algorithms
- Parallel fitness evaluation for larger playlists
- Genre-specific BPM mixing rules

## Related

- [beets](https://beets.io/) - Music library manager
- [Camelot Wheel](https://mixedinkey.com/harmonic-mixing-guide/) - Harmonic mixing guide