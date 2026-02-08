package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// applyGamePatches applies any built-in patches for the given game profile.
// gameFilesDir is the directory containing the extracted game files.
// patchesDir is the path to the extracted embedded patches directory (may be empty).
func applyGamePatches(gameFilesDir, patchesDir string, profile GameProfile, r ProgressReporter) error {
	switch profile.Slug {
	case "civnet":
		return applyCivNetPatches(gameFilesDir, patchesDir, r)
	default:
		return nil
	}
}

// applyCivNetPatches applies the official 1.02 patch and widescreen fix to CivNet.
func applyCivNetPatches(gameFilesDir, patchesDir string, r ProgressReporter) error {
	// Step 1: Apply official 1.02 patch (copy updated files over game dir)
	civnetPatchDir := filepath.Join(patchesDir, "civnet")
	if _, err := os.Stat(civnetPatchDir); err == nil {
		r.Log("  Applying CivNet v1.02 patch...")
		entries, err := os.ReadDir(civnetPatchDir)
		if err != nil {
			return fmt.Errorf("read patch dir: %w", err)
		}
		count := 0
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			// Skip the patch readme
			if name == "patch.txt" {
				continue
			}
			src := filepath.Join(civnetPatchDir, name)
			// Match case-insensitively against existing game files
			dst := findCaseInsensitive(gameFilesDir, name)
			if dst == "" {
				// File doesn't exist yet, just copy with original name
				dst = filepath.Join(gameFilesDir, name)
			}
			if err := copyFile(src, dst); err != nil {
				return fmt.Errorf("apply patch file %s: %w", name, err)
			}
			count++
		}
		r.Logf("  Applied %d patch files (v1.02)", count)
	} else {
		r.Log("  Warning: CivNet 1.02 patch files not found, skipping")
	}

	// Step 2: Apply widescreen hex fix to civnet.exe
	// The widescreen patch changes the max window size from 1300x1048 to 32000x32000
	// by modifying 5 bytes at offset 0x147cff in the v1.02 civnet.exe:
	//   Old: 18 04 68 14 05  (push 1048; push 1300)
	//   New: 00 7d 68 00 7d  (push 32000; push 32000)
	exePath := findCaseInsensitive(gameFilesDir, "civnet.exe")
	if exePath == "" {
		return fmt.Errorf("civnet.exe not found in game files")
	}

	r.Log("  Applying widescreen patch...")
	if err := applyWidescreenPatch(exePath); err != nil {
		r.Logf("  Warning: widescreen patch failed: %v", err)
		// Non-fatal - game still works without it
		return nil
	}
	r.Log("  Applied widescreen patch (max window: 32000x32000)")

	return nil
}

// applyWidescreenPatch modifies civnet.exe v1.02 to support widescreen resolutions.
func applyWidescreenPatch(exePath string) error {
	const offset = 0x147cff
	oldBytes := []byte{0x18, 0x04, 0x68, 0x14, 0x05}
	newBytes := []byte{0x00, 0x7d, 0x68, 0x00, 0x7d}

	data, err := os.ReadFile(exePath)
	if err != nil {
		return fmt.Errorf("read exe: %w", err)
	}

	// Verify file size (v1.02 is exactly 2,073,600 bytes)
	if len(data) != 2073600 {
		return fmt.Errorf("unexpected exe size %d (expected 2073600 for v1.02)", len(data))
	}

	// Verify the expected bytes are present
	if offset+len(oldBytes) > len(data) {
		return fmt.Errorf("offset 0x%x out of range", offset)
	}

	for i, b := range oldBytes {
		if data[offset+i] != b {
			// Check if already patched
			alreadyPatched := true
			for j, nb := range newBytes {
				if data[offset+j] != nb {
					alreadyPatched = false
					break
				}
			}
			if alreadyPatched {
				return nil // Already patched
			}
			return fmt.Errorf("byte mismatch at offset 0x%x+%d: expected 0x%02x, got 0x%02x (not v1.02?)",
				offset, i, b, data[offset+i])
		}
	}

	// Apply the patch
	copy(data[offset:], newBytes)

	if err := os.WriteFile(exePath, data, 0755); err != nil {
		return fmt.Errorf("write patched exe: %w", err)
	}

	return nil
}

// findCaseInsensitive finds a file in dir matching name case-insensitively.
// Returns the full path if found, empty string otherwise.
func findCaseInsensitive(dir, name string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	lowerName := toLower(name)
	for _, e := range entries {
		if toLower(e.Name()) == lowerName {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}
