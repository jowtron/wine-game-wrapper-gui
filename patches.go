package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// applyGamePatches applies the profile's declarative patches to the installed
// game directory: in-tree merges first, then overlay file sets, then hex
// patches.
func applyGamePatches(gameDir, patchesDir string, profile GameProfile, r ProgressReporter) error {
	for _, m := range profile.Merges {
		if err := applyMerge(gameDir, m, r); err != nil {
			return err
		}
	}
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

// applyMerge copies one subtree of the installed game dir onto another and
// removes the source. This models installer staging dirs: e.g. innoextract
// dumps files a GOG Inno installer would place into the install tree under
// __support/save/, so Civ3 merges "__support/save" -> "." (LSANS.TTF,
// conquests.biq, ...). The staging dir is deleted after the merge so the
// bundle doesn't ship duplicate copies.
func applyMerge(gameDir string, m DirMerge, r ProgressReporter) error {
	src := filepath.Join(gameDir, filepath.FromSlash(m.From))
	if _, err := os.Stat(src); os.IsNotExist(err) {
		r.Logf("  Warning: merge source %q not found, skipping", m.From)
		return nil
	}
	dst := gameDir
	if m.To != "" && m.To != "." {
		dst = filepath.Join(gameDir, filepath.FromSlash(m.To))
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return fmt.Errorf("create merge dest %s: %w", dst, err)
		}
	}
	count, err := overlayDir(src, dst, nil)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(src); err != nil {
		return fmt.Errorf("remove merged staging dir %s: %w", src, err)
	}
	r.Logf("  Merged %q into %q (%d files)", m.From, m.To, count)
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

	count, err := overlayDir(srcDir, gameDir, skip)
	if err != nil {
		return err
	}
	r.Logf("  Applied overlay %q (%d files)", o.Source, count)
	return nil
}

// overlayDir recursively copies srcDir's files onto gameDir, matching existing
// names (files and directories) case-insensitively. Returns the file count.
func overlayDir(srcDir, gameDir string, skip map[string]bool) (int, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return 0, fmt.Errorf("read patch dir %s: %w", srcDir, err)
	}
	count := 0
	for _, e := range entries {
		if skip[strings.ToLower(e.Name())] {
			continue
		}
		src := filepath.Join(srcDir, e.Name())
		dst := findCaseInsensitive(gameDir, e.Name())
		if e.IsDir() {
			if dst == "" {
				dst = filepath.Join(gameDir, e.Name())
				if err := os.MkdirAll(dst, 0o755); err != nil {
					return count, fmt.Errorf("create patch dir %s: %w", dst, err)
				}
			}
			n, err := overlayDir(src, dst, skip)
			count += n
			if err != nil {
				return count, err
			}
			continue
		}
		if dst == "" {
			dst = filepath.Join(gameDir, e.Name())
		}
		if err := copyFile(src, dst); err != nil {
			return count, fmt.Errorf("apply patch file %s: %w", e.Name(), err)
		}
		count++
	}
	return count, nil
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

// findCaseInsensitivePath resolves a relative path (slash- or
// backslash-separated) under dir, matching every component case-insensitively.
// Returns the full path if found, empty string otherwise.
func findCaseInsensitivePath(dir, rel string) string {
	cur := dir
	for _, part := range strings.Split(strings.ReplaceAll(rel, "\\", "/"), "/") {
		if part == "" || part == "." {
			continue
		}
		cur = findCaseInsensitive(cur, part)
		if cur == "" {
			return ""
		}
	}
	return cur
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
