// ABOUTME: Handles reading and writing M3U8 playlist files
// ABOUTME: Provides functions to load playlists with metadata and save sorted playlists back to disk

// Package playlist handles M3U8 playlist files and music metadata.
// It reads playlists, extracts metadata directly from audio file tags (ID3, Vorbis, etc.),
// and provides harmonic mixing utilities based on the Camelot wheel system.
package playlist

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ReadPlaylist reads an M3U8 playlist file and fetches metadata for all tracks
// Returns a slice of Track structs with full metadata
func ReadPlaylist(path string) ([]Track, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open playlist: %w", err)
	}

	defer func() {
		_ = file.Close() // Explicitly ignore error for read-only file
	}()

	var tracks []Track

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		tracks = append(tracks, Track{Path: line})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading playlist: %w", err)
	}

	return tracks, nil
}

// LoadPlaylistWithMetadata reads a playlist and fetches metadata from beets for each track
// Tracks that fail to load metadata are filtered out and not included in the result
// Displays progress as it fetches metadata for each track if verbose is true
func LoadPlaylistWithMetadata(path string, verbose bool) ([]Track, error) {
	tracks, err := ReadPlaylist(path)
	if err != nil {
		return nil, err
	}

	if verbose {
		fmt.Printf("Loading metadata for %d tracks...\n", len(tracks))
	}

	// Fetch metadata for each track, filtering out failures
	validTracks := make([]Track, 0, len(tracks))
	skippedCount := 0

	for i := range tracks {
		if verbose && (i+1)%10 == 0 {
			fmt.Printf("[+] Processed %d/%d tracks...\n", i+1, len(tracks))
		}

		metadata, err := GetTrackMetadata(tracks[i].Path)
		if err != nil {
			if verbose {
				fmt.Printf("[!] Skipping track (could not load metadata): %s: %v\n", tracks[i].Path, err)
			}

			skippedCount++

			continue
		}

		// Add successfully loaded track
		validTracks = append(validTracks, *metadata)
	}

	return validTracks, nil
}

// WritePlaylist writes a slice of tracks to an M3U8 playlist file
// Only writes the Path field of each track (not metadata)
// Creates a backup (.bak) of the existing file before overwriting
func WritePlaylist(path string, tracks []Track) error {
	// Create backup if file exists
	if _, err := os.Stat(path); err == nil {
		backupPath := path + ".bak"
		if err := os.Rename(path, backupPath); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create playlist: %w", err)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close playlist file: %w", closeErr)
		}
	}()

	writer := bufio.NewWriter(file)
	for _, track := range tracks {
		if _, err := writer.WriteString(track.Path + "\n"); err != nil {
			return fmt.Errorf("failed to write track: %w", err)
		}
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush writer: %w", err)
	}

	return nil
}
