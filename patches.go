package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// applyGamePatches applies the profile's declarative patches to the installed
// game directory: overlay file sets first, then hex patches.
func applyGamePatches(gameDir, patchesDir string, profile GameProfile, r ProgressReporter) error {
	for _, o := range profile.Overlays {
		if err := applyOverlay(gameDir, patchesDir, o, r); err != nil {
			return err
		}
	}
	for _, h := range profile.HexPatches {
		if err := applyHexPatch(gameDir, h, r); err != nil {
			if h.Optional {
				r.Logf("  Warning: %s: %v (continuing)", h.describe(), err)
				continue
			}
			return err
		}
	}
	return nil
}

// applyOverlay copies the files of resources/patches/<Source> over the game
// directory, matching existing filenames case-insensitively.
func applyOverlay(gameDir, patchesDir string, o OverlayPatch, r ProgressReporter) error {
	// Resolve the patch files: embedded, then user-supplied build-inputs, then
	// download (for sources we can't bundle for licensing reasons, e.g. civnet).
	srcDir := resolveOverlaySource(patchesDir, o.Source, r)
	if srcDir == "" {
		r.Logf("  Warning: patch files for %q not found, skipping", o.Source)
		return nil
	}

	skip := map[string]bool{}
	for _, s := range o.Skip {
		skip[strings.ToLower(s)] = true
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("read patch dir %s: %w", srcDir, err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || skip[strings.ToLower(e.Name())] {
			continue
		}
		src := filepath.Join(srcDir, e.Name())
		dst := findCaseInsensitive(gameDir, e.Name())
		if dst == "" {
			dst = filepath.Join(gameDir, e.Name())
		}
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("apply patch file %s: %w", e.Name(), err)
		}
		count++
	}
	r.Logf("  Applied overlay %q (%d files)", o.Source, count)
	return nil
}

func (h HexPatch) describe() string {
	if h.Desc != "" {
		return h.Desc
	}
	return fmt.Sprintf("hex patch %s@0x%x", h.File, h.Offset)
}

// applyHexPatch applies an in-place byte patch with verification.
// It is idempotent: if the replace bytes are already present, it succeeds.
func applyHexPatch(gameDir string, h HexPatch, r ProgressReporter) error {
	path := findCaseInsensitive(gameDir, h.File)
	if path == "" {
		return fmt.Errorf("%s not found in game files", h.File)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", h.File, err)
	}

	if h.ExpectSize > 0 && int64(len(data)) != h.ExpectSize {
		return fmt.Errorf("unexpected size %d for %s (expected %d — wrong version?)", len(data), h.File, h.ExpectSize)
	}
	end := h.Offset + int64(len(h.Expect))
	if h.Offset < 0 || end > int64(len(data)) {
		return fmt.Errorf("offset 0x%x out of range for %s", h.Offset, h.File)
	}

	matches := func(want []int) bool {
		for i, b := range want {
			if data[h.Offset+int64(i)] != byte(b) {
				return false
			}
		}
		return true
	}

	if matches(h.Replace) {
		r.Logf("  %s: already applied", h.describe())
		return nil
	}
	if !matches(h.Expect) {
		return fmt.Errorf("byte mismatch at 0x%x in %s (wrong version?)", h.Offset, h.File)
	}

	for i, b := range h.Replace {
		data[h.Offset+int64(i)] = byte(b)
	}
	if err := os.WriteFile(path, data, 0755); err != nil {
		return fmt.Errorf("write patched %s: %w", h.File, err)
	}
	r.Logf("  Applied %s", h.describe())
	return nil
}

// findCaseInsensitive finds a file in dir matching name case-insensitively.
// Returns the full path if found, empty string otherwise.
func findCaseInsensitive(dir, name string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	lowerName := strings.ToLower(name)
	for _, e := range entries {
		if strings.ToLower(e.Name()) == lowerName {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}
