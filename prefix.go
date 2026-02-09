package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// initWinePrefix creates and initializes a Wine prefix using wineboot.
// If win16 is true, sets the Windows version to win95 for 16-bit app compatibility.
func initWinePrefix(wineBin, prefixDir string, win16 bool, r ProgressReporter) error {
	if err := os.MkdirAll(prefixDir, 0755); err != nil {
		return fmt.Errorf("create prefix dir: %w", err)
	}

	wineboot := filepath.Join(filepath.Dir(wineBin), "wineboot")

	cmd := exec.Command(wineboot, "--init")
	cmd.Env = append(os.Environ(),
		"WINEPREFIX="+prefixDir,
		"WINEDEBUG=-all",
	)
	// Suppress verbose Wine/Vulkan output - only show our own log messages

	r.Log("  Running wineboot --init (this may take a moment)...")
	if out, err := cmd.CombinedOutput(); err != nil {
		r.Log(string(out)) // Show output only on error
		return fmt.Errorf("wineboot --init failed: %w", err)
	}

	// Configure registry: disable Mono dialog, set Windows version for Win16
	wine := filepath.Join(filepath.Dir(wineBin), "wine")
	regFile, err := os.CreateTemp("", "prefix-config-*.reg")
	if err == nil {
		var reg strings.Builder
		reg.WriteString("REGEDIT4\n\n")
		reg.WriteString("[HKEY_CURRENT_USER\\Software\\Wine\\DllOverrides]\n")
		reg.WriteString("\"mscoree\"=\"\"\n")
		reg.WriteString("\n")
		if win16 {
			r.Log("  Setting Windows version to win95 (required for Win16 apps)...")
			reg.WriteString("[HKEY_CURRENT_USER\\Software\\Wine]\n")
			reg.WriteString("\"Version\"=\"win95\"\n")
		}
		// Enable Wine virtual desktop so the game runs in a managed window
		// with macOS title bar (close/minimize/fullscreen buttons)
		reg.WriteString("\n[HKEY_CURRENT_USER\\Software\\Wine\\Explorer\\Desktops]\n")
		reg.WriteString("\"Default\"=\"1920x1080\"\n")
		regFile.WriteString(reg.String())
		regFile.Close()
		regCmd := exec.Command(wine, "regedit", regFile.Name())
		regCmd.Env = append(os.Environ(),
			"WINEPREFIX="+prefixDir,
			"WINEDEBUG=-all",
		)
		regCmd.Run()
		os.Remove(regFile.Name())
	}
	r.Log("  Configured registry settings")

	// Kill wineserver after init
	wineserver := filepath.Join(filepath.Dir(wineBin), "wineserver")
	kill := exec.Command(wineserver, "-k")
	kill.Env = append(os.Environ(), "WINEPREFIX="+prefixDir)
	kill.Run() // ignore errors

	return nil
}

// installMcicda copies the mcicda.dll to both system32 and syswow64 in the prefix.
func installMcicda(dllPath, prefixDir string, r ProgressReporter) error {
	dests := []string{
		filepath.Join(prefixDir, "drive_c", "windows", "system32", "mcicda.dll"),
		filepath.Join(prefixDir, "drive_c", "windows", "syswow64", "mcicda.dll"),
	}

	for _, dest := range dests {
		dir := filepath.Dir(dest)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue // syswow64 may not exist on 32-bit prefix
		}
		if err := copyFile(dllPath, dest); err != nil {
			return fmt.Errorf("copy mcicda.dll to %s: %w", dest, err)
		}
		r.Logf("  Installed mcicda.dll -> %s", dest)
	}
	return nil
}

// installOtvdm copies otvdm files to the Wine prefix for Win16 support.
// It also registers DLL overrides so Wine uses the native (otvdm) versions.
func installOtvdm(otvdmDir, wineBin, prefixDir string, r ProgressReporter) error {
	destDir := filepath.Join(prefixDir, "drive_c", "windows", "syswow64")
	if _, err := os.Stat(destDir); os.IsNotExist(err) {
		destDir = filepath.Join(prefixDir, "drive_c", "windows", "system32")
	}

	entries, err := os.ReadDir(otvdmDir)
	if err != nil {
		return fmt.Errorf("read otvdm dir: %w", err)
	}

	var overrideNames []string
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		src := filepath.Join(otvdmDir, name)
		dst := filepath.Join(destDir, name)
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("copy %s: %w", name, err)
		}
		count++
		// Track 16-bit modules for DLL overrides
		if strings.HasSuffix(name, ".exe16") || strings.HasSuffix(name, ".dll16") ||
			strings.HasSuffix(name, ".drv16") || strings.HasSuffix(name, ".mod16") ||
			name == "wow32.dll" {
			overrideNames = append(overrideNames, name)
		}
	}

	r.Logf("  Installed %d otvdm files -> %s", count, destDir)

	// Generate otvdm.ini with PeekMessageSleep to reduce CPU from busy-wait loops.
	// Win16 games often poll PeekMessage in a tight loop; this adds a 1ms sleep per call.
	iniPath := filepath.Join(destDir, "otvdm.ini")
	os.WriteFile(iniPath, []byte("[otvdm]\nPeekMessageSleep=1\n"), 0644)
	r.Log("  Installed otvdm.ini (PeekMessageSleep=1)")

	// Register DLL overrides so Wine uses native (otvdm) versions
	if err := registerOtvdmOverrides(wineBin, prefixDir, overrideNames, r); err != nil {
		return fmt.Errorf("register DLL overrides: %w", err)
	}

	return nil
}

// registerOtvdmOverrides sets Wine DLL overrides for all otvdm 16-bit files.
func registerOtvdmOverrides(wineBin, prefixDir string, names []string, r ProgressReporter) error {
	var reg strings.Builder
	reg.WriteString("REGEDIT4\n\n")
	reg.WriteString("[HKEY_CURRENT_USER\\Software\\Wine\\DllOverrides]\n")

	for _, name := range names {
		key := "*" + name
		reg.WriteString(fmt.Sprintf("\"%s\"=\"native,builtin\"\n", key))
	}
	reg.WriteString("\"*wow32\"=\"native,builtin\"\n")

	regFile, err := os.CreateTemp("", "otvdm-overrides-*.reg")
	if err != nil {
		return err
	}
	defer os.Remove(regFile.Name())

	if _, err := regFile.WriteString(reg.String()); err != nil {
		regFile.Close()
		return err
	}
	regFile.Close()

	wine := filepath.Join(filepath.Dir(wineBin), "wine")
	cmd := exec.Command(wine, "regedit", regFile.Name())
	cmd.Env = append(os.Environ(),
		"WINEPREFIX="+prefixDir,
		"WINEDEBUG=-all",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		r.Log(string(out))
		return fmt.Errorf("wine regedit failed: %w", err)
	}

	wineserver := filepath.Join(filepath.Dir(wineBin), "wineserver")
	kill := exec.Command(wineserver, "-k")
	kill.Env = append(os.Environ(), "WINEPREFIX="+prefixDir)
	kill.Run()

	r.Logf("  Registered %d DLL overrides for otvdm", len(names)+1)
	return nil
}

// installGameFiles copies game files and music into the prefix.
func installGameFiles(gameFilesDir, musicDir, prefixDir string, profile GameProfile, r ProgressReporter) error {
	// Copy game files
	gameDestDir := filepath.Join(prefixDir, "drive_c", profile.GameDir)
	if err := os.MkdirAll(gameDestDir, 0755); err != nil {
		return err
	}

	r.Logf("  Copying game files to %s...", profile.GameDir)
	cmd := exec.Command("cp", "-R", gameFilesDir+"/.", gameDestDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copy game files: %w", err)
	}

	// Copy music (FLAC files)
	musicDestDir := filepath.Join(prefixDir, "drive_c", "music")
	if err := os.MkdirAll(musicDestDir, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(musicDir)
	if err != nil {
		return fmt.Errorf("read music dir: %w", err)
	}

	count := 0
	for _, e := range entries {
		name := e.Name()
		lower := toLower(name)
		if !hasAnySuffix(lower, ".flac", ".wav", ".mp3", ".ogg", ".opus") {
			continue
		}
		src := filepath.Join(musicDir, name)
		dst := filepath.Join(musicDestDir, name)
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("copy %s: %w", name, err)
		}
		count++
	}

	r.Logf("  Installed %d audio tracks", count)
	return nil
}

// copyFile copies a single file, preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		result[i] = c
	}
	return string(result)
}

func hasAnySuffix(s string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix {
			return true
		}
	}
	return false
}
