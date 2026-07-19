package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// buildApp assembles the .app bundle from all prepared components.
// embeddedResourcesDir is the path to extracted embedded resources (icons, etc).
// cdromDir, if non-empty and present, is bundled as Resources/cdrom and mapped
// as drive d: for games that read from the CD at runtime.
func buildApp(appPath string, profile GameProfile, wineDir, prefixDir, embeddedResourcesDir, cdromDir string, r ProgressReporter) error {
	// Create .app directory structure
	contentsDir := filepath.Join(appPath, "Contents")
	macosDir := filepath.Join(contentsDir, "MacOS")
	resourcesDir := filepath.Join(contentsDir, "Resources")

	for _, dir := range []string{macosDir, resourcesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}

	// Write Info.plist
	plistPath := filepath.Join(contentsDir, "Info.plist")
	if err := os.WriteFile(plistPath, []byte(generateInfoPlist(profile)), 0644); err != nil {
		return fmt.Errorf("write Info.plist: %w", err)
	}
	r.Log("  Wrote Info.plist")

	// Write launcher script
	launcherPath := filepath.Join(macosDir, "launch")
	if err := os.WriteFile(launcherPath, []byte(generateLauncherScript(profile)), 0755); err != nil {
		return fmt.Errorf("write launcher: %w", err)
	}
	r.Log("  Wrote launcher script")

	// Copy Wine installation
	wineDestDir := filepath.Join(resourcesDir, "wine")
	r.Log("  Copying Wine installation (this may take a moment)...")
	cmd := exec.Command("cp", "-R", wineDir, wineDestDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copy Wine: %w", err)
	}

	// Strip Wine in the bundle copy
	if err := stripWine(wineDestDir, r); err != nil {
		r.Logf("  Warning: could not strip Wine: %v", err)
	}

	// Copy Wine prefix
	prefixDestDir := filepath.Join(resourcesDir, "wineprefix")
	r.Log("  Copying Wine prefix...")
	cmd = exec.Command("cp", "-R", prefixDir, prefixDestDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copy prefix: %w", err)
	}

	// Move the game directory out of the prefix to Resources/game/<GameDir>.
	// It is the pristine master copy: the launcher seeds a per-user live copy
	// in ~/Library/Application Support on first run and symlinks it into
	// drive_c, so saves survive bundle replacement.
	gameSrc := filepath.Join(prefixDestDir, "drive_c", profile.GameDir)
	gameMaster := filepath.Join(resourcesDir, "game")
	if err := os.MkdirAll(gameMaster, 0755); err != nil {
		return fmt.Errorf("create game master dir: %w", err)
	}
	if err := os.Rename(gameSrc, filepath.Join(gameMaster, profile.GameDir)); err != nil {
		return fmt.Errorf("move game dir to master: %w", err)
	}
	r.Logf("  Moved %s to Resources/game (saves will live in Application Support)", profile.GameDir)

	// Copy retained CD content (drive d:) if the game needs it
	haveCDROM := false
	if cdromDir != "" {
		if _, err := os.Stat(cdromDir); err == nil {
			r.Log("  Copying retained CD content...")
			cmd := exec.Command("cp", "-R", cdromDir, filepath.Join(resourcesDir, "cdrom"))
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("copy CD content: %w", err)
			}
			haveCDROM = true
		}
	}

	// Fix dosdevices to use relative paths
	dosdevicesDir := filepath.Join(prefixDestDir, "dosdevices")
	os.MkdirAll(dosdevicesDir, 0755)
	// Remove existing symlinks and recreate with relative paths
	os.Remove(filepath.Join(dosdevicesDir, "c:"))
	os.Remove(filepath.Join(dosdevicesDir, "z:"))
	os.Remove(filepath.Join(dosdevicesDir, "d:"))
	os.Symlink("../drive_c", filepath.Join(dosdevicesDir, "c:"))
	os.Symlink("/", filepath.Join(dosdevicesDir, "z:"))
	if haveCDROM {
		os.Symlink("../../cdrom", filepath.Join(dosdevicesDir, "d:"))
	}

	// Install app icon if available
	iconDst := filepath.Join(resourcesDir, "AppIcon.icns")
	if embeddedResourcesDir != "" {
		iconSrc := filepath.Join(embeddedResourcesDir, "icons", profile.Slug+".icns")
		if _, err := os.Stat(iconSrc); err == nil {
			if err := copyFile(iconSrc, iconDst); err == nil {
				r.Logf("  Installed app icon")
			}
		}
	}

	// Set custom icon on Wine binary so it shows our icon in the dock
	// (our .app uses LSUIElement to hide itself from the dock, so only
	// Wine's process is visible - give it the game icon)
	// The actual process that creates windows is in lib/wine/x86_64-unix/wine,
	// not bin/wine (which is just a launcher)
	wineUnixBin := filepath.Join(wineDestDir, "lib", "wine", "x86_64-unix", "wine")
	if _, err := os.Stat(wineUnixBin); err != nil {
		// Fallback to bin/wine if unix binary not found
		wineUnixBin = filepath.Join(wineDestDir, "bin", "wine")
	}
	if _, err := os.Stat(iconDst); err == nil {
		if err := setFileIcon(iconDst, wineUnixBin); err != nil {
			r.Logf("  Warning: could not set Wine dock icon: %v", err)
		} else {
			r.Log("  Set custom dock icon on Wine binary")
		}
	}

	return nil
}

// setFileIcon sets a custom Finder icon on a file using NSWorkspace.
func setFileIcon(icnsPath, targetPath string) error {
	script := fmt.Sprintf(`
import AppKit
guard let icon = NSImage(contentsOfFile: %q) else { exit(1) }
if !NSWorkspace.shared.setIcon(icon, forFile: %q, options: []) { exit(2) }
`, icnsPath, targetPath)
	cmd := exec.Command("swift", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(out))
	}
	return nil
}

// dirSize returns the total size of a directory tree in bytes.
func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// formatSize formats bytes as a human-readable string.
func formatSize(bytes int64) string {
	const (
		MB = 1024 * 1024
		GB = 1024 * 1024 * 1024
	)
	if bytes >= GB {
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	}
	return fmt.Sprintf("%.0f MB", float64(bytes)/float64(MB))
}
