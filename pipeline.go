package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BuildConfig holds all configuration for a build, received from the frontend.
type BuildConfig struct {
	GameSlug   string `json:"gameSlug"`
	CustomExe  string `json:"customExe"`
	CuePath    string `json:"cuePath"`
	OutputPath string `json:"outputPath"`
	WinePath   string `json:"winePath"`
	OtvdmPath  string `json:"otvdmPath"`
	McicdaPath string `json:"mcicdaPath"`
	Win16      bool   `json:"win16"`
}

// ensurePATH adds common Homebrew and system paths to PATH.
// macOS .app bundles don't inherit the user's shell PATH, so tools like
// bchunk, flac, and hdiutil may not be found without this.
func ensurePATH() {
	extraPaths := []string{
		"/usr/local/bin",
		"/opt/homebrew/bin",
		"/opt/homebrew/sbin",
		"/usr/bin",
		"/bin",
		"/usr/sbin",
		"/sbin",
	}
	current := os.Getenv("PATH")
	for _, p := range extraPaths {
		if !strings.Contains(current, p) {
			current = current + ":" + p
		}
	}
	os.Setenv("PATH", current)
}

// runPipeline executes the 7-step build pipeline, reporting progress via r.
func runPipeline(config BuildConfig, r ProgressReporter) error {
	ensurePATH()

	// Resolve game profile
	var profile GameProfile
	if config.GameSlug != "" && config.GameSlug != "custom" {
		var ok bool
		profile, ok = LookupProfile(config.GameSlug)
		if !ok {
			return fmt.Errorf("unknown game profile: %s", config.GameSlug)
		}
	} else {
		if config.CustomExe == "" {
			return fmt.Errorf("custom exe name is required when no game profile is selected")
		}
		profile = CustomProfile(config.CustomExe)
	}

	if config.Win16 {
		profile.Win16 = true
	}

	// Resolve CUE/BIN paths
	absQue, err := filepath.Abs(config.CuePath)
	if err != nil {
		return fmt.Errorf("resolve CUE path: %w", err)
	}
	if _, err := os.Stat(absQue); err != nil {
		return fmt.Errorf("CUE file not found: %s", absQue)
	}

	binPath, err := resolveBINPath(absQue)
	if err != nil {
		return fmt.Errorf("resolve BIN file: %w", err)
	}

	// Extract embedded resources (mcicda.dll, otvdm, patches)
	resourcesDir, err := extractEmbeddedResources()
	if err != nil {
		return fmt.Errorf("extract embedded resources: %w", err)
	}
	defer os.RemoveAll(resourcesDir)

	// Resolve mcicda.dll path
	dllPath := config.McicdaPath
	if dllPath == "" {
		dllPath = filepath.Join(resourcesDir, "mcicda.dll")
	}
	if _, err := os.Stat(dllPath); err != nil {
		return fmt.Errorf("mcicda.dll not found: %s", dllPath)
	}

	// Resolve output path
	appPath := config.OutputPath
	if appPath == "" {
		appPath = filepath.Join("/Applications", profile.Name+".app")
	}
	// Ensure .app extension
	if !strings.HasSuffix(appPath, ".app") {
		appPath += ".app"
	}
	appPath, _ = filepath.Abs(appPath)

	// Check if output already exists — caller should have handled this via overwrite prompt
	if _, err := os.Stat(appPath); err == nil {
		return fmt.Errorf("output already exists: %s", appPath)
	}

	// Print banner
	r.Log("==========================================")
	r.Logf("wine-game-wrapper GUI")
	r.Log("==========================================")
	r.Logf("Game:     %s", profile.Name)
	r.Logf("Exe:      %s", profile.Exe)
	r.Logf("Win16:    %v", profile.Win16)
	r.Logf("CUE:      %s", absQue)
	r.Logf("BIN:      %s", binPath)
	r.Logf("Output:   %s", appPath)
	r.Log("")

	// Create temp working directory
	tmpDir, err := os.MkdirTemp("", "wine-game-wrapper-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tracksDir := filepath.Join(tmpDir, "tracks")
	gameFilesDir := filepath.Join(tmpDir, "game")
	musicDir := filepath.Join(tmpDir, "music")
	prefixDir := filepath.Join(tmpDir, "prefix")

	// Step 1: Obtain Wine
	r.Step(1, 7, "Obtaining Wine...")
	wineDir, err := prepareWine(config.WinePath, r)
	if err != nil {
		return fmt.Errorf("obtain Wine: %w", err)
	}
	wineBin := filepath.Join(wineDir, "bin", "wine")
	r.Logf("  Wine: %s", wineDir)

	// Step 2: Extract CUE/BIN
	r.Step(2, 7, "Extracting CUE/BIN tracks...")
	if err := extractTracks(binPath, absQue, tracksDir, r); err != nil {
		return fmt.Errorf("extract tracks: %w", err)
	}

	// Step 3: Extract game files from data track
	r.Step(3, 7, "Extracting game files from ISO...")
	isoPath, err := findDataTrackISO(tracksDir)
	if err != nil {
		return fmt.Errorf("find data track: %w", err)
	}
	if err := extractGameFiles(isoPath, gameFilesDir, r); err != nil {
		return fmt.Errorf("extract game files: %w", err)
	}

	// Apply game patches (e.g. CivNet 1.02 patch + widescreen fix)
	patchesDir := findPatchesDir(resourcesDir)
	if patchesDir != "" {
		r.Log("  Applying game patches...")
		if err := applyGamePatches(gameFilesDir, patchesDir, profile, r); err != nil {
			return fmt.Errorf("apply patches: %w", err)
		}
	}

	// Step 4: Convert audio to FLAC
	r.Step(4, 7, "Converting audio tracks to FLAC...")
	if err := os.MkdirAll(musicDir, 0755); err != nil {
		return fmt.Errorf("create music dir: %w", err)
	}
	entries, err := os.ReadDir(tracksDir)
	if err != nil {
		return fmt.Errorf("read tracks dir: %w", err)
	}
	for _, e := range entries {
		name := e.Name()
		lower := toLower(name)
		if hasAnySuffix(lower, ".wav") && !hasAnySuffix(lower, "track01.wav") {
			src := filepath.Join(tracksDir, name)
			dst := filepath.Join(musicDir, name)
			if err := os.Rename(src, dst); err != nil {
				if err := copyFile(src, dst); err != nil {
					return fmt.Errorf("move %s: %w", name, err)
				}
				os.Remove(src)
			}
		}
	}
	trackCount, err := convertToFLAC(musicDir, r)
	if err != nil {
		return fmt.Errorf("convert to FLAC: %w", err)
	}
	r.Logf("  Converted %d audio tracks", trackCount)

	// Step 5: Initialize Wine prefix
	r.Step(5, 7, "Initializing Wine prefix...")
	if err := initWinePrefix(wineBin, prefixDir, profile, r); err != nil {
		return fmt.Errorf("init Wine prefix: %w", err)
	}

	// Step 6: Install components into prefix
	r.Step(6, 7, "Installing game components...")
	if err := installMcicda(dllPath, prefixDir, r); err != nil {
		return fmt.Errorf("install mcicda.dll: %w", err)
	}

	if profile.Win16 {
		otvdm := config.OtvdmPath
		if otvdm == "" {
			otvdm = findOtvdmDir(resourcesDir)
			if otvdm == "" {
				r.Log("  Warning: otvdm not found - Win16 game may not run")
				r.Log("  Use Advanced Options to specify the otvdm build directory")
			}
		}
		if otvdm != "" {
			if err := installOtvdm(otvdm, wineBin, prefixDir, r); err != nil {
				return fmt.Errorf("install otvdm: %w", err)
			}
		}
	}

	if err := installGameFiles(gameFilesDir, musicDir, prefixDir, profile, r); err != nil {
		return fmt.Errorf("install game files: %w", err)
	}

	if err := installKeyremap(resourcesDir, prefixDir, profile, r); err != nil {
		return fmt.Errorf("install keyremap: %w", err)
	}

	// Step 7: Assemble .app bundle
	r.Step(7, 7, "Building .app bundle...")
	if err := buildApp(appPath, profile, wineDir, prefixDir, resourcesDir, r); err != nil {
		return fmt.Errorf("build .app: %w", err)
	}

	// Report success
	size, _ := dirSize(appPath)
	r.Log("")
	r.Log("==========================================")
	r.Log("Build complete!")
	r.Log("==========================================")
	r.Logf("Output: %s", appPath)
	r.Logf("Size:   %s", formatSize(size))

	return nil
}

// findPatchesDir searches for game patch files in standard locations.
func findPatchesDir(resourcesDir string) string {
	candidates := []string{}
	exePath, _ := os.Executable()
	if exePath != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(exePath), "resources", "patches"))
	}
	candidates = append(candidates, filepath.Join("resources", "patches"))
	if resourcesDir != "" {
		candidates = append(candidates, filepath.Join(resourcesDir, "patches"))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// findOtvdmDir searches for otvdm files in standard locations.
// resourcesDir is the path to extracted embedded resources (may be empty).
func findOtvdmDir(resourcesDir string) string {
	candidates := []string{}

	exePath, _ := os.Executable()
	if exePath != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(exePath), "resources", "otvdm"))
	}
	candidates = append(candidates, filepath.Join("resources", "otvdm"))
	if resourcesDir != "" {
		candidates = append(candidates, filepath.Join(resourcesDir, "otvdm"))
	}
	if exePath != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(exePath), "..", "otvdm-build"))
	}
	candidates = append(candidates, filepath.Join("..", "otvdm-build"))

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}
