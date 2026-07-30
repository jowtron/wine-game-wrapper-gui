package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// externalPatchSources maps an overlay `source` name to a download URL for
// patch sets that are NOT bundled in the repo for licensing reasons — i.e.
// copyrighted game binaries we can't redistribute. At build time such a source
// is resolved (see resolveOverlaySource) from, in order:
//
//  1. the embedded resources/patches/<source> (if it was bundled after all)
//  2. build-inputs/patches/<source>  — a copy the user supplies themselves
//  3. a download from the URL below, cached under ~/.cache/wine-game-wrapper
//
// The download URL can rot; the build-inputs copy is the durable fallback.
var externalPatchSources = map[string]string{
	// Sid Meier's CivNet v1.0.2 (Patch 3): the full patched game binaries
	// (Civnet.exe + language DLLs). Freely distributed historically but still
	// copyrighted (2K/MicroProse), so not committed to the repo.
	"civnet": "https://archive.org/download/civnetp3/civnetp3.zip",
}

// externalPatchDir is where a user drops a self-supplied patch set.
func externalPatchDir(source string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support", "wine-game-wrapper",
		"build-inputs", "patches", source)
}

// resolveOverlaySource returns a directory holding the overlay's patch files,
// or "" if it can't be resolved (the caller then skips with a warning).
func resolveOverlaySource(patchesDir, source string, r ProgressReporter) string {
	// 1. Embedded (bundled in the app)
	if patchesDir != "" {
		if d := filepath.Join(patchesDir, source); dirHasFiles(d) {
			return d
		}
	}
	// 2. User-provided under build-inputs (durable, offline)
	if d := externalPatchDir(source); dirHasFiles(d) {
		r.Logf("  Using %q patch from build-inputs", source)
		return d
	}
	// 3. Download + cache
	url, ok := externalPatchSources[source]
	if !ok {
		return ""
	}
	cacheDir, err := getCacheDir()
	if err != nil {
		return ""
	}
	dest := filepath.Join(cacheDir, "patches", source)
	if dirHasFiles(dest) {
		r.Logf("  Using cached %q patch", source)
		return dest
	}
	r.Logf("  Downloading %q patch (%s)...", source, url)
	if err := downloadAndUnzip(url, dest, r); err != nil {
		os.RemoveAll(dest)
		r.Logf("  Warning: could not download %q patch: %v", source, err)
		r.Logf("  Supply it manually in %s and rebuild", externalPatchDir(source))
		return ""
	}
	return dest
}

// dirHasFiles reports whether dir exists and contains at least one regular file.
func dirHasFiles(dir string) bool {
	if dir == "" {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			return true
		}
		if dirHasFiles(filepath.Join(dir, e.Name())) {
			return true
		}
	}
	return false
}

// downloadAndUnzip fetches a .zip URL and extracts its files, flattened, into
// destDir. Entries are written by base name only, which also neutralises any
// zip-slip path traversal.
func downloadAndUnzip(url, destDir string, r ProgressReporter) error {
	tmp, err := os.CreateTemp("", "wgw-patch-*.zip")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	if err := downloadFile(url, tmpPath, r); err != nil {
		return err
	}
	zr, err := zip.OpenReader(tmpPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.Base(f.Name)
		if name == "" || name == "." || name == ".." {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(filepath.Join(destDir, name))
		if err != nil {
			rc.Close()
			return err
		}
		_, cerr := io.Copy(out, rc)
		out.Close()
		rc.Close()
		if cerr != nil {
			return cerr
		}
	}
	return nil
}
