// ABOUTME: Tests for M3U8 playlist reading and writing
// ABOUTME: Verifies file I/O, comment handling, and empty file edge cases

package playlist

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReadPlaylist verifies M3U8 parsing
func TestReadPlaylist(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		expectCount int
		expectError bool
	}{
		{
			name: "simple playlist",
			content: `Artist/Album/01 Track.mp3
Artist/Album/02 Track.mp3
Artist/Album/03 Track.mp3`,
			expectCount: 3,
			expectError: false,
		},
		{
			name: "with comments",
			content: `#EXTM3U
# This is a comment
Artist/Album/01 Track.mp3
# Another comment
Artist/Album/02 Track.mp3`,
			expectCount: 2,
			expectError: false,
		},
		{
			name: "with empty lines",
			content: `Artist/Album/01 Track.mp3

Artist/Album/02 Track.mp3

`,
			expectCount: 2,
			expectError: false,
		},
		{
			name:        "empty file",
			content:     "",
			expectCount: 0,
			expectError: false,
		},
		{
			name: "only comments",
			content: `#EXTM3U
# Just comments
# No tracks`,
			expectCount: 0,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "test.m3u8")

			if err := os.WriteFile(tmpFile, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			// Test ReadPlaylist
			tracks, err := ReadPlaylist(tmpFile)

			if tt.expectError && err == nil {
				t.Error("Expected error, got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if len(tracks) != tt.expectCount {
				t.Errorf("Expected %d tracks, got %d", tt.expectCount, len(tracks))
			}

			// Verify track paths are set
			for i, track := range tracks {
				if track.Path == "" {
					t.Errorf("Track %d has empty path", i)
				}
			}
		})
	}
}

// TestReadPlaylistNonExistent verifies error handling for missing files
func TestReadPlaylistNonExistent(t *testing.T) {
	tracks, err := ReadPlaylist("/nonexistent/path/to/playlist.m3u8")

	if err == nil {
		t.Error("Expected error for nonexistent file, got none")
	}

	if len(tracks) != 0 {
		t.Errorf("Expected 0 tracks for failed read, got %d", len(tracks))
	}
}

// TestWritePlaylist verifies M3U8 writing
func TestWritePlaylist(t *testing.T) {
	tests := []struct {
		name   string
		tracks []Track
	}{
		{
			name: "simple tracks",
			tracks: []Track{
				{Path: "Artist/Album/01 Track.mp3"},
				{Path: "Artist/Album/02 Track.mp3"},
				{Path: "Artist/Album/03 Track.mp3"},
			},
		},
		{
			name:   "empty tracks",
			tracks: []Track{},
		},
		{
			name: "single track",
			tracks: []Track{
				{Path: "Artist/Album/01 Track.mp3"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "test.m3u8")

			// Write playlist
			err := WritePlaylist(tmpFile, tt.tracks)
			if err != nil {
				t.Fatalf("Failed to write playlist: %v", err)
			}

			// Read it back
			readTracks, err := ReadPlaylist(tmpFile)
			if err != nil {
				t.Fatalf("Failed to read playlist: %v", err)
			}

			// Verify count matches
			if len(readTracks) != len(tt.tracks) {
				t.Errorf("Expected %d tracks after write/read, got %d", len(tt.tracks), len(readTracks))
			}

			// Verify paths match
			for i := range tt.tracks {
				if readTracks[i].Path != tt.tracks[i].Path {
					t.Errorf("Track %d: expected path %s, got %s", i, tt.tracks[i].Path, readTracks[i].Path)
				}
			}
		})
	}
}

// TestWritePlaylistInvalidPath verifies error handling for invalid paths
func TestWritePlaylistInvalidPath(t *testing.T) {
	tracks := []Track{
		{Path: "Artist/Album/01 Track.mp3"},
	}

	// Try to write to invalid directory
	err := WritePlaylist("/nonexistent/directory/playlist.m3u8", tracks)

	if err == nil {
		t.Error("Expected error for invalid path, got none")
	}
}

// TestRoundTrip verifies write then read preserves data
func TestRoundTrip(t *testing.T) {
	tracks := []Track{
		{Path: "Fred V & Grafix/Oxygen/01 Ignite.mp3"},
		{Path: "BCee & Charlotte Haining/Life as We Know It/04 The Hills.mp3"},
		{Path: "Calibre/Spill/02 Running.mp3"},
	}

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "roundtrip.m3u8")

	// Write
	if err := WritePlaylist(tmpFile, tracks); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read
	readTracks, err := ReadPlaylist(tmpFile)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// Verify
	if len(readTracks) != len(tracks) {
		t.Fatalf("Expected %d tracks, got %d", len(tracks), len(readTracks))
	}

	for i := range tracks {
		if readTracks[i].Path != tracks[i].Path {
			t.Errorf("Track %d path mismatch: expected %s, got %s", i, tracks[i].Path, readTracks[i].Path)
		}
	}
}
