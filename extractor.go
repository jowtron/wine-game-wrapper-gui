package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// parseBINFromCUE reads a CUE file and returns the BIN filename referenced in it.
func parseBINFromCUE(cuePath string) (string, error) {
	f, err := os.Open(cuePath)
	if err != nil {
		return "", fmt.Errorf("open CUE file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Look for: FILE "something.bin" BINARY
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "FILE") {
			// Extract filename between quotes
			start := strings.Index(line, "\"")
			if start < 0 {
				continue
			}
			end := strings.Index(line[start+1:], "\"")
			if end < 0 {
				continue
			}
			return line[start+1 : start+1+end], nil
		}
	}
	return "", fmt.Errorf("no FILE directive found in CUE file")
}

// resolveBINPath finds the BIN file path from a CUE file.
// First tries <cuefile>.bin (replacing .cue extension), then parses the CUE.
func resolveBINPath(cuePath string) (string, error) {
	// Try replacing .cue with .bin
	ext := filepath.Ext(cuePath)
	candidate := cuePath[:len(cuePath)-len(ext)] + ".bin"
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	// Also try .BIN
	candidate = cuePath[:len(cuePath)-len(ext)] + ".BIN"
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}

	// Parse CUE to get filename
	binName, err := parseBINFromCUE(cuePath)
	if err != nil {
		return "", err
	}
	candidate = filepath.Join(filepath.Dir(cuePath), binName)
	if _, err := os.Stat(candidate); err != nil {
		return "", fmt.Errorf("BIN file not found: %s", candidate)
	}
	return candidate, nil
}

// extractTracks splits a CUE/BIN into individual tracks using pure Go.
func extractTracks(binPath, cuePath, outputDir string, r ProgressReporter) error {
	return splitCUEBIN(binPath, cuePath, outputDir, r)
}

// extractGameFiles mounts an ISO and copies game files to destDir.
func extractGameFiles(isoPath, destDir string, r ProgressReporter) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create game dir: %w", err)
	}

	// Create a temp mount point
	mountPoint, err := os.MkdirTemp("", "wine-game-iso-")
	if err != nil {
		return fmt.Errorf("create mount point: %w", err)
	}
	defer os.RemoveAll(mountPoint)

	// Mount the ISO
	r.Logf("  Mounting %s...", filepath.Base(isoPath))
	cmd := exec.Command("hdiutil", "mount", "-mountpoint", mountPoint, "-nobrowse", isoPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hdiutil mount failed: %w", err)
	}

	// Ensure we unmount when done
	defer func() {
		detach := exec.Command("hdiutil", "detach", mountPoint, "-force")
		detach.Run()
	}()

	// Copy all files from mounted volume
	r.Logf("  Copying game files...")
	cmd = exec.Command("cp", "-R", mountPoint+"/.", destDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copy game files failed: %w", err)
	}

	return nil
}

// findDataTrackISO finds the ISO file (track01) in the extraction output directory.
func findDataTrackISO(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		name := strings.ToLower(e.Name())
		if strings.HasSuffix(name, ".iso") {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("no ISO data track found in %s", dir)
}
