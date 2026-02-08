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
ln -sf ../drive_c "$PREFIX/dosdevices/c:"
ln -sf / "$PREFIX/dosdevices/z:"

# Kill any stale wineserver for this prefix and wait for it to fully exit
WINEPREFIX="$PREFIX" "$WINESERVER" -k 2>/dev/null || true
WINEPREFIX="$PREFIX" "$WINESERVER" --wait 2>/dev/null || true

export WINEPREFIX="$PREFIX"
export WINEDLLOVERRIDES="mcicda=n"
export WINEDEBUG=-all
export DYLD_FALLBACK_LIBRARY_PATH="$APP_DIR/Resources/wine/lib"

# Get screen resolution for Wine virtual desktop
SCREEN_RES=$(system_profiler SPDisplaysDataType 2>/dev/null | \
    grep -i "Resolution:" | head -1 | \
    sed 's/.*: *\([0-9]*\) *x *\([0-9]*\).*/\1x\2/')
if [ -z "$SCREEN_RES" ]; then
    SCREEN_RES="1920x1080"
fi

# Use Wine virtual desktop so macOS window chrome (close/minimize/fullscreen
# buttons) is available, and the game can be Cmd-Tabbed away from
cd "$PREFIX/drive_c/%s"
"$WINE" explorer /desktop=game,${SCREEN_RES} %s

# Wait for wineserver to exit (keeps the .app alive while game runs)
"$WINESERVER" --wait 2>/dev/null
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
