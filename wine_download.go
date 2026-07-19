package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	gcenxReleasesAPI = "https://api.github.com/repos/Gcenx/macOS_Wine_builds/releases"
	cacheSubdir      = "wine-game-wrapper"
)

// getCacheDir returns the cache directory for downloaded Wine archives.
func getCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".cache", cacheSubdir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// findSystemWine checks for a system Wine installation that can be copied.
func findSystemWine() string {
	// Check Wine Stable.app
	appPath := "/Applications/Wine Stable.app/Contents/Resources/wine"
	if info, err := os.Stat(filepath.Join(appPath, "bin", "wine")); err == nil && !info.IsDir() {
		return appPath
	}

	// Check common Homebrew locations
	brewPaths := []string{
		"/usr/local/opt/wine-stable",
		"/opt/homebrew/opt/wine-stable",
	}
	for _, p := range brewPaths {
		binPath := filepath.Join(p, "bin", "wine")
		if info, err := os.Stat(binPath); err == nil && !info.IsDir() {
			// Resolve the prefix (Homebrew cellar layout)
			real, err := filepath.EvalSymlinks(p)
			if err == nil {
				return real
			}
			return p
		}
	}

	return ""
}

// ghRelease represents a subset of the GitHub release API response.
type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// findStableAsset returns the best wine-stable tar.xz asset from a release.
func findStableAsset(release ghRelease) *ghAsset {
	arch := runtime.GOARCH
	var fallback *ghAsset
	for i, asset := range release.Assets {
		name := strings.ToLower(asset.Name)
		if !strings.HasSuffix(name, ".tar.xz") || !strings.Contains(name, "wine-stable") {
			continue
		}
		if strings.Contains(name, "universal") ||
			strings.Contains(name, arch) ||
			strings.Contains(name, "osx64") {
			return &release.Assets[i]
		}
		if fallback == nil {
			fallback = &release.Assets[i]
		}
	}
	return fallback
}

// findWineDownloadURL queries the gcenx GitHub releases API for the latest stable Wine archive.
func findWineDownloadURL() (string, string, error) {
	resp, err := http.Get(gcenxReleasesAPI)
	if err != nil {
		return "", "", fmt.Errorf("fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var releases []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", "", fmt.Errorf("parse releases JSON: %w", err)
	}

	// Find the first release that contains a wine-stable asset
	for _, release := range releases {
		if asset := findStableAsset(release); asset != nil {
			return asset.BrowserDownloadURL, asset.Name, nil
		}
	}

	return "", "", fmt.Errorf("no wine-stable release found")
}

// downloadFile downloads a URL to a local path, reporting progress.
func downloadFile(url, destPath string, r ProgressReporter) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	size := resp.ContentLength
	written := int64(0)
	buf := make([]byte, 32*1024)
	lastPct := -1

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			written += int64(n)
			if size > 0 {
				pct := int(written * 100 / size)
				if pct != lastPct && pct%10 == 0 {
					r.Logf("  Downloaded %d%%", pct)
					lastPct = pct
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}

	return nil
}

// downloadWine downloads and extracts Wine from gcenx releases, caching the result.
// Returns the path to the extracted Wine directory.
func downloadWine(r ProgressReporter) (string, error) {
	cacheDir, err := getCacheDir()
	if err != nil {
		return "", err
	}

	// Check cache for existing extraction
	extractedDir := filepath.Join(cacheDir, "wine")
	wineBin := filepath.Join(extractedDir, "bin", "wine")
	if _, err := os.Stat(wineBin); err == nil {
		r.Log("  Using cached Wine from ~/.cache/wine-game-wrapper/wine")
		return extractedDir, nil
	}

	r.Log("  Finding latest Wine release from Gcenx/macOS_Wine_builds...")
	url, filename, err := findWineDownloadURL()
	if err != nil {
		return "", fmt.Errorf("find download URL: %w", err)
	}

	archivePath := filepath.Join(cacheDir, filename)

	// Download if not cached
	if _, err := os.Stat(archivePath); err != nil {
		r.Logf("  Downloading %s...", filename)
		if err := downloadFile(url, archivePath, r); err != nil {
			return "", fmt.Errorf("download Wine: %w", err)
		}
	} else {
		r.Logf("  Using cached archive: %s", filename)
	}

	// Extract
	r.Log("  Extracting Wine archive...")
	tmpExtract, err := os.MkdirTemp(cacheDir, "extract-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpExtract)

	cmd := exec.Command("tar", "xf", archivePath, "-C", tmpExtract)
	cmd.Stderr = r.Writer()
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("extract archive: %w", err)
	}

	// Find the Wine directory inside the extraction
	winePath, err := findWineInDir(tmpExtract)
	if err != nil {
		return "", err
	}

	// Move to final cache location
	os.RemoveAll(extractedDir)
	if err := os.Rename(winePath, extractedDir); err != nil {
		// Rename across filesystems - fall back to copy
		cmd := exec.Command("cp", "-R", winePath, extractedDir)
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("copy extracted Wine: %w", err)
		}
	}

	return extractedDir, nil
}

// findWineInDir searches for the wine binary in an extracted directory tree.
// Handles both flat layouts (wine/bin/wine) and .app bundles
// (Wine Stable.app/Contents/Resources/wine/bin/wine).
func findWineInDir(dir string) (string, error) {
	// Check if wine binary is directly at dir/bin/wine
	if _, err := os.Stat(filepath.Join(dir, "bin", "wine")); err == nil {
		return dir, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidate := filepath.Join(dir, e.Name())

		// Check candidate/bin/wine (flat layout)
		if _, err := os.Stat(filepath.Join(candidate, "bin", "wine")); err == nil {
			return candidate, nil
		}

		// Check .app bundle: candidate/Contents/Resources/wine/bin/wine
		appWine := filepath.Join(candidate, "Contents", "Resources", "wine")
		if _, err := os.Stat(filepath.Join(appWine, "bin", "wine")); err == nil {
			return appWine, nil
		}

		// Check one more level (e.g. "Wine Stable/wine/")
		sub, _ := os.ReadDir(candidate)
		for _, s := range sub {
			if !s.IsDir() {
				continue
			}
			deep := filepath.Join(candidate, s.Name())
			if _, err := os.Stat(filepath.Join(deep, "bin", "wine")); err == nil {
				return deep, nil
			}
		}
	}

	return "", fmt.Errorf("could not find wine binary in extracted archive at %s", dir)
}

// stripWine removes unnecessary components from a Wine installation to reduce size.
func stripWine(wineDir string, r ProgressReporter) error {
	// Remove Gecko (IE engine) - ~207MB
	geckoDir := filepath.Join(wineDir, "share", "wine", "gecko")
	if info, err := os.Stat(geckoDir); err == nil && info.IsDir() {
		r.Log("  Removing Wine Gecko (IE engine)...")
		os.RemoveAll(geckoDir)
	}

	// Remove Mono (.NET) - ~87MB
	monoDir := filepath.Join(wineDir, "share", "wine", "mono")
	if info, err := os.Stat(monoDir); err == nil && info.IsDir() {
		r.Log("  Removing Wine Mono (.NET)...")
		os.RemoveAll(monoDir)
	}

	return nil
}

// prepareWine obtains a Wine installation, either from a provided path,
// the system, or by downloading. Returns the path to the Wine directory.
func prepareWine(userPath string, r ProgressReporter) (string, error) {
	if userPath != "" {
		// User specified a path
		if _, err := os.Stat(filepath.Join(userPath, "bin", "wine")); err != nil {
			return "", fmt.Errorf("wine binary not found at %s/bin/wine", userPath)
		}
		return userPath, nil
	}

	// Check system Wine
	if sysWine := findSystemWine(); sysWine != "" {
		r.Logf("  Found system Wine at %s", sysWine)
		return sysWine, nil
	}

	// Download
	return downloadWine(r)
}
