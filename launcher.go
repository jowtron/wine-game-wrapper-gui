package main

import "fmt"

// generateLauncherScript returns the bash launcher script content for the .app bundle.
func generateLauncherScript(profile GameProfile) string {
	exe := profile.Exe
	gameDir := profile.GameDir

	return fmt.Sprintf(`#!/bin/bash
#
# Auto-generated launcher for %s
# Created by wine-game-wrapper
#

APP_DIR="$(cd "$(dirname "$0")/.." && pwd)"
WINE="$APP_DIR/Resources/wine/bin/wine"
WINESERVER="$APP_DIR/Resources/wine/bin/wineserver"
PREFIX="$APP_DIR/Resources/wineprefix"

# Fix dosdevices symlinks (they break when .app is moved)
mkdir -p "$PREFIX/dosdevices"
rm -f "$PREFIX/dosdevices/c:" "$PREFIX/dosdevices/z:" "$PREFIX/dosdevices/d:"
ln -s ../drive_c "$PREFIX/dosdevices/c:"
ln -s / "$PREFIX/dosdevices/z:"
# Map bundled CD content as drive d: (games that read videos/music from CD)
if [ -d "$APP_DIR/Resources/cdrom" ]; then
    ln -s ../../cdrom "$PREFIX/dosdevices/d:"
fi

# Force kill all Wine processes belonging to this app.
# Finds wineserver by path, then kills its children (winedevice.exe etc.)
# which don't show the Wine path in their command line.
kill_wine_procs() {
    local pids
    pids=$(pgrep -f "$APP_DIR/Resources/wine" 2>/dev/null)
    for pid in $pids; do
        pkill -9 -P "$pid" 2>/dev/null
        kill -9 "$pid" 2>/dev/null
    done
}

# Kill any stale Wine processes from a previous run
kill_wine_procs

export WINEPREFIX="$PREFIX"
export WINEDLLOVERRIDES="mcicda=n;keyremap=n"
export WINEDEBUG=-all
export DYLD_FALLBACK_LIBRARY_PATH="$APP_DIR/Resources/wine/lib"

# Clean up on exit: graceful shutdown, then force kill stragglers
cleanup() {
    "$WINESERVER" -k 2>/dev/null
    sleep 2
    kill_wine_procs
}
trap cleanup EXIT
trap 'exit 1' INT TERM HUP

# Start key remapping hook if keyhook.exe and keyremap.ini exist
if [ -f "$PREFIX/drive_c/keyhook.exe" ] && [ -f "$PREFIX/drive_c/keyremap.ini" ]; then
    "$WINE" "C:\keyhook.exe" &
    sleep 1
fi

# Launch game in background so signals can interrupt 'wait' and fire the trap.
# (Foreground commands block trap delivery in bash.)
cd "$PREFIX/drive_c/%s"
nice -n 19 "$WINE" %s &
GAME_PID=$!

# Wait only for the game process — once it exits, clean up and quit.
# ('wait $pid' is interruptible by signals, unlike foreground commands.)
wait $GAME_PID 2>/dev/null
`, profile.Name, gameDir, exe)
}

// generateInfoPlist returns the Info.plist XML content for the .app bundle.
func generateInfoPlist(profile GameProfile) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key>
    <string>%s</string>
    <key>CFBundleDisplayName</key>
    <string>%s</string>
    <key>CFBundleIdentifier</key>
    <string>%s</string>
    <key>CFBundleExecutable</key>
    <string>launch</string>
    <key>CFBundleVersion</key>
    <string>1.0</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>LSUIElement</key>
    <true/>
</dict>
</plist>
`, profile.Name, profile.Name, profile.BundleID)
}
