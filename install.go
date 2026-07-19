package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// installStrategy describes how game files get from the CD into the prefix.
//
//	copy-cd-root              copy the whole data track (CivNet-style run-from-CD)
//	copy-dir:<path>           copy one directory from the CD
//	extract-installshield     extract data1.cab with unshield (optionally :<dir> on CD)
//	run-installer:<path>      run the CD's installer under Wine (interactive)
type installStrategy struct {
	Kind string
	Arg  string
}

var validStrategies = map[string]bool{
	"copy-cd-root":          true,
	"copy-dir":              true,
	"extract-installshield": true,
	"run-installer":         true,
}

// parseInstallStrategy parses an install string like "copy-dir:INSTALL".
func parseInstallStrategy(s string) (installStrategy, error) {
	kind, arg, _ := strings.Cut(s, ":")
	if !validStrategies[kind] {
		return installStrategy{}, fmt.Errorf("unknown install strategy %q", kind)
	}
	if (kind == "copy-dir" || kind == "run-installer") && arg == "" {
		return installStrategy{}, fmt.Errorf("install strategy %q requires an argument, e.g. %q", kind, kind+":INSTALL")
	}
	return installStrategy{Kind: kind, Arg: arg}, nil
}

// installGameFromCD mounts the CD data track and installs the game into the
// prefix according to the profile's install strategy, then applies patches
// and stages any retain_cd directories into cdromDir.
// The prefix must already be initialized (run-installer needs it).
func installGameFromCD(isoPath string, profile GameProfile, wineBin, prefixDir, resourcesDir, cdromDir string, r ProgressReporter) error {
	strategy, err := parseInstallStrategy(profile.Install)
	if err != nil {
		return err
	}

	// Mount the data track
	mountPoint, err := os.MkdirTemp("", "wine-game-iso-")
	if err != nil {
		return fmt.Errorf("create mount point: %w", err)
	}
	defer os.RemoveAll(mountPoint)

	r.Logf("  Mounting %s...", filepath.Base(isoPath))
	cmd := exec.Command("hdiutil", "mount", "-mountpoint", mountPoint, "-nobrowse", "-readonly", isoPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hdiutil mount failed: %w", err)
	}
	defer func() {
		detach := exec.Command("hdiutil", "detach", mountPoint, "-force")
		detach.Run()
	}()

	gameDest := filepath.Join(prefixDir, "drive_c", profile.GameDir)

	switch strategy.Kind {
	case "copy-cd-root":
		r.Log("  Copying CD contents...")
		if err := copyTree(mountPoint, gameDest); err != nil {
			return fmt.Errorf("copy game files: %w", err)
		}

	case "copy-dir":
		src := filepath.Join(mountPoint, filepath.FromSlash(strategy.Arg))
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("install source %q not found on CD", strategy.Arg)
		}
		r.Logf("  Copying %s from CD...", strategy.Arg)
		if err := copyTree(src, gameDest); err != nil {
			return fmt.Errorf("copy game files: %w", err)
		}

	case "extract-installshield":
		if err := extractInstallShield(mountPoint, strategy.Arg, gameDest, r); err != nil {
			return err
		}

	case "run-installer":
		if err := runInstaller(mountPoint, strategy.Arg, profile, wineBin, prefixDir, r); err != nil {
			return err
		}
	}

	// Expand any ARCV-compressed files (EDI Install Pro archives, e.g.
	// Colonization's COLONIZE.$00 -> colonize.exe)
	if err := expandARCVFiles(gameDest, r); err != nil {
		return fmt.Errorf("expand ARCV archives: %w", err)
	}

	// Sanity check: the game exe must exist after installation
	if findCaseInsensitive(gameDest, profile.Exe) == "" {
		return fmt.Errorf("%s not found in %s after install — check the profile's game_dir and install strategy", profile.Exe, gameDest)
	}

	// Apply declarative patches
	if err := applyGamePatches(gameDest, findPatchesDir(resourcesDir), profile, r); err != nil {
		return fmt.Errorf("apply patches: %w", err)
	}

	// Stage retain_cd directories for bundling as drive d:
	if len(profile.RetainCD) > 0 {
		if err := stageRetainCD(mountPoint, profile, cdromDir, r); err != nil {
			return fmt.Errorf("stage retained CD content: %w", err)
		}
	}

	return nil
}

// extractInstallShield extracts an InstallShield 5 data1.cab using unshield.
func extractInstallShield(mountPoint, cabDir, gameDest string, r ProgressReporter) error {
	unshield, err := exec.LookPath("unshield")
	if err != nil {
		return fmt.Errorf("this game needs 'unshield' to extract its InstallShield archive.\nInstall it with: brew install unshield")
	}

	cab := filepath.Join(mountPoint, filepath.FromSlash(cabDir), "data1.cab")
	if _, err := os.Stat(cab); err != nil {
		return fmt.Errorf("data1.cab not found on CD (looked in %q)", filepath.Dir(cab))
	}

	if err := os.MkdirAll(gameDest, 0755); err != nil {
		return err
	}

	r.Log("  Extracting InstallShield archive (unshield)...")
	cmd := exec.Command(unshield, "-d", gameDest, "x", cab)
	if out, err := cmd.CombinedOutput(); err != nil {
		r.Log(string(out))
		return fmt.Errorf("unshield failed: %w", err)
	}

	// unshield extracts into per-component subdirectories; if the exe isn't
	// at the top level, flatten single-component layouts up one level.
	flattenExtractedComponents(gameDest, r)
	return nil
}

// flattenExtractedComponents moves files up from unshield's per-component
// directories when the game's files are nested one level down.
func flattenExtractedComponents(gameDest string, r ProgressReporter) {
	entries, err := os.ReadDir(gameDest)
	if err != nil {
		return
	}
	// Only flatten when the top level contains directories and no files
	var dirs []os.DirEntry
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e)
		} else {
			return // files already at top level, leave as-is
		}
	}
	for _, d := range dirs {
		sub := filepath.Join(gameDest, d.Name())
		subEntries, err := os.ReadDir(sub)
		if err != nil {
			continue
		}
		for _, se := range subEntries {
			src := filepath.Join(sub, se.Name())
			dst := filepath.Join(gameDest, se.Name())
			if _, err := os.Stat(dst); err == nil {
				continue // don't clobber
			}
			os.Rename(src, dst)
		}
		// Remove the component dir if now empty
		if remaining, err := os.ReadDir(sub); err == nil && len(remaining) == 0 {
			os.Remove(sub)
		}
	}
	r.Log("  Flattened extracted components")
}

// runInstaller runs the CD's installer under Wine, with the CD visible as d:.
// This is interactive: the user clicks through the installer window.
func runInstaller(mountPoint, installerPath string, profile GameProfile, wineBin, prefixDir string, r ProgressReporter) error {
	installer := filepath.Join(mountPoint, filepath.FromSlash(installerPath))
	if _, err := os.Stat(installer); err != nil {
		return fmt.Errorf("installer %q not found on CD", installerPath)
	}

	// Point d: at the mounted CD so the installer sees a CD-ROM drive
	dosdevices := filepath.Join(prefixDir, "dosdevices")
	os.MkdirAll(dosdevices, 0755)
	dLink := filepath.Join(dosdevices, "d:")
	os.Remove(dLink)
	if err := os.Symlink(mountPoint, dLink); err != nil {
		return fmt.Errorf("map d: to CD: %w", err)
	}
	defer os.Remove(dLink)

	r.Log("")
	r.Logf("  >>> The %s installer will now open in a window.", profile.Name)
	r.Logf("  >>> Install to the default location (C:\\%s) and finish the installer.", profile.GameDir)
	r.Log("  >>> The build continues automatically when the installer exits.")
	r.Log("")

	cmd := exec.Command(wineBin, installer)
	cmd.Dir = filepath.Dir(installer)
	cmd.Env = append(os.Environ(),
		"WINEPREFIX="+prefixDir,
		"WINEDEBUG=-all",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		r.Log(string(out))
		return fmt.Errorf("installer failed: %w", err)
	}

	// Let Wine settle, then kill the server so the prefix is quiescent
	wineserver := filepath.Join(filepath.Dir(wineBin), "wineserver")
	wait := exec.Command(wineserver, "--wait")
	wait.Env = append(os.Environ(), "WINEPREFIX="+prefixDir)
	wait.Run()

	return nil
}

// stageRetainCD copies the profile's retain_cd paths from the mounted CD into
// cdromDir, preserving their relative layout, and writes the volume label.
func stageRetainCD(mountPoint string, profile GameProfile, cdromDir string, r ProgressReporter) error {
	for _, rel := range profile.RetainCD {
		src := filepath.Join(mountPoint, filepath.FromSlash(rel))
		info, err := os.Stat(src)
		if err != nil {
			return fmt.Errorf("retain_cd path %q not found on CD", rel)
		}
		dst := filepath.Join(cdromDir, filepath.FromSlash(rel))
		r.Logf("  Retaining CD content: %s", rel)
		if info.IsDir() {
			err = copyTree(src, dst)
		} else {
			if mkErr := os.MkdirAll(filepath.Dir(dst), 0755); mkErr != nil {
				return mkErr
			}
			err = copyFile(src, dst)
		}
		if err != nil {
			return fmt.Errorf("copy %s: %w", rel, err)
		}
	}

	// Wine reads the volume label of a directory-backed drive from this file
	if profile.CDLabel != "" {
		labelPath := filepath.Join(cdromDir, ".windows-label")
		if err := os.WriteFile(labelPath, []byte(profile.CDLabel+"\n"), 0644); err != nil {
			return fmt.Errorf("write volume label: %w", err)
		}
	}
	return nil
}

// copyTree copies the contents of src into dst (creating dst).
func copyTree(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	cmd := exec.Command("cp", "-R", src+"/.", dst)
	return cmd.Run()
}

// expandARCVFiles finds ARCV archives (EDI Install Pro compressed files,
// typically *.$00) in dir, decompresses each to its original filename, and
// removes the archive. Non-ARCV files are left untouched.
func expandARCVFiles(dir string, r ProgressReporter) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil || !IsARCV(data) {
			continue
		}
		f, err := ParseARCV(data)
		if err != nil {
			r.Logf("  Warning: %s looks like ARCV but failed to parse: %v", e.Name(), err)
			continue
		}
		out, err := f.Decompress()
		if err != nil {
			return fmt.Errorf("decompress %s: %w", e.Name(), err)
		}
		dst := filepath.Join(dir, f.Name)
		if err := os.WriteFile(dst, out, 0755); err != nil {
			return fmt.Errorf("write %s: %w", f.Name, err)
		}
		os.Remove(path)
		r.Logf("  Expanded ARCV: %s -> %s (%d bytes, CRC verified)", e.Name(), f.Name, len(out))
	}
	return nil
}
