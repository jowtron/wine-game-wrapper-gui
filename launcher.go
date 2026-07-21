package main

import "fmt"

// generateLauncherScript returns the bash launcher script content for the .app bundle.
//
// Two architectures, chosen by profile.Win16:
//
//   - Win32: the script ends with `exec wine <abs path to exe>` so the game
//     runs AS the .app's process. LaunchServices then shows one Dock tile
//     with the bundle's icon and name, and winemac can activate/front the
//     game window (Info.plist ships LSUIElement=false for these). Cleanup
//     can't use traps (the shell is gone after exec), so a watchdog subshell
//     forked before exec waits for the process to die and then shuts Wine
//     down. WINEARCH=wow64 is required: without it ntdll decides a 32-bit
//     main exe needs the (absent) i386 loader and reroutes through
//     start.exe, which puts the game in a different process.
//
//   - Win16: the game runs via otvdm in separate Wine processes, so exec
//     can't adopt the bundle identity. The launcher stays an invisible
//     accessory app (LSUIElement=true) that spawns wine as a child, waits,
//     and cleans up via traps. winemac can't read NE-format icons, so the
//     Finder icon set on the Wine binary shows in the Dock.
func generateLauncherScript(profile GameProfile) string {
	head := fmt.Sprintf(`#!/bin/bash
#
# Auto-generated launcher for %s
# Created by wine-game-wrapper
#

APP_DIR="$(cd "$(dirname "$0")/.." && pwd)"
WINE="$APP_DIR/Resources/wine/bin/wine"
WINESERVER="$APP_DIR/Resources/wine/bin/wineserver"
PREFIX="$APP_DIR/Resources/wineprefix"
GAME_DIR=%q
DATA_DIR="$HOME/Library/Application Support/wine-game-wrapper/%s"

# Fix dosdevices symlinks (they break when .app is moved). Also drop any
# d:: DEVICE link: mountmgr matches real volumes (e.g. mounted disk
# images) against it and rebinds d: to them, stamping the drive type
# "floppy" in the registry and breaking CD checks. With d:: absent and
# the d: mount link present, mountmgr leaves the letter alone.
mkdir -p "$PREFIX/dosdevices"
rm -f "$PREFIX/dosdevices/c:" "$PREFIX/dosdevices/z:" \
      "$PREFIX/dosdevices/d:" "$PREFIX/dosdevices/d::"
ln -s ../drive_c "$PREFIX/dosdevices/c:"
ln -s / "$PREFIX/dosdevices/z:"
# Map bundled CD content as drive d: (games that read videos/music from CD)
if [ -d "$APP_DIR/Resources/cdrom" ]; then
    ln -s ../../cdrom "$PREFIX/dosdevices/d:"
fi

# Game files live outside the bundle so saves survive app replacement.
# Seed the live copy from the bundle's pristine master on first run.
if [ ! -d "$DATA_DIR/$GAME_DIR" ]; then
    mkdir -p "$DATA_DIR"
    echo "First run: copying game files to $DATA_DIR..."
    cp -R "$APP_DIR/Resources/game/$GAME_DIR" "$DATA_DIR/$GAME_DIR"
fi
# Symlink the game dir into the prefix (replace a real dir from old bundles)
if [ ! -L "$PREFIX/drive_c/$GAME_DIR" ]; then
    rm -rf "$PREFIX/drive_c/$GAME_DIR"
    ln -s "$DATA_DIR/$GAME_DIR" "$PREFIX/drive_c/$GAME_DIR"
fi

# Force kill all Wine processes belonging to this app.
# Finds wineserver by path and kills its children, then sweeps up
# Wine-spawned services (explorer.exe, winedevice.exe, ...): those show
# only C:\ paths in ps and reparent to launchd, so they are identified
# by which wine binary they have mapped (lsof).
kill_wine_procs() {
    local pid
    for pid in $(pgrep -f "$APP_DIR/Resources/wine" 2>/dev/null); do
        pkill -9 -P "$pid" 2>/dev/null
        kill -9 "$pid" 2>/dev/null
    done
    for pid in $(pgrep -f '[A-Za-z]:\\' 2>/dev/null); do
        if lsof -p "$pid" 2>/dev/null | grep -qF "$APP_DIR/Resources/wine"; then
            kill -9 "$pid" 2>/dev/null
        fi
    done
}

# Kill any stale Wine processes from a previous run
kill_wine_procs

export WINEPREFIX="$PREFIX"
export WINEDLLOVERRIDES="mcicda=n;keyremap=n"
export WINEDEBUG=-all
export DYLD_FALLBACK_LIBRARY_PATH="$APP_DIR/Resources/wine/lib"

# Auto theme: apply the Windows palette matching the current macOS appearance.
# (Static dark/light themes are already baked into the prefix at build time.)
#
# This runs with a bounded wait: a wedged Wine/wineserver must never hang the
# whole app launch (that shows as "can't open ... not responding"). If the
# regedit doesn't finish in time we kill it and launch the game anyway (worst
# case: the game starts unthemed this once). We do NOT kill wineserver here —
# the game reuses this warm server instead of paying a second cold start.
if [ -d "$APP_DIR/Resources/themes" ]; then
    if defaults read -g AppleInterfaceStyle 2>/dev/null | grep -qi dark; then
        MODE=dark
    else
        MODE=light
    fi
    "$WINE" regedit "$APP_DIR/Resources/themes/$MODE.reg" >/dev/null 2>&1 &
    THEME_PID=$!
    for _ in $(seq 1 20); do
        kill -0 "$THEME_PID" 2>/dev/null || break
        sleep 1
    done
    kill -9 "$THEME_PID" 2>/dev/null
fi

# Start key remapping hook if keyhook.exe and keyremap.ini exist
if [ -f "$PREFIX/drive_c/keyhook.exe" ] && [ -f "$PREFIX/drive_c/keyremap.ini" ]; then
    "$WINE" "C:\keyhook.exe" &
    sleep 1
fi
`, profile.Name, profile.GameDir, profile.Slug)

	if profile.Win16 {
		return head + fmt.Sprintf(`
# Clean up on exit: graceful shutdown, then force kill stragglers
cleanup() {
    "$WINESERVER" -k 2>/dev/null
    sleep 2
    kill_wine_procs
}
trap cleanup EXIT
trap 'exit 1' INT TERM HUP

# Launch game in background so signals can interrupt 'wait' and fire the trap.
# (Foreground commands block trap delivery in bash.)
cd "$PREFIX/drive_c/$GAME_DIR"
nice -n 19 "$WINE" %s &
GAME_PID=$!

# Wait only for the game process — once it exits, clean up and quit.
# ('wait $pid' is interruptible by signals, unlike foreground commands.)
wait $GAME_PID 2>/dev/null
`, profile.Exe)
	}

	return head + fmt.Sprintf(`
# See the generator comment: force new-wow64 so the 32-bit exe loads in
# THIS process instead of being rerouted through start.exe.
export WINEARCH=wow64

# The game replaces this script via exec below, so this shell can't clean
# up with traps. A watchdog subshell forked before exec survives it, waits
# for this PID to die, then shuts Wine down.
LAUNCH_PID=$$
(
    while kill -0 "$LAUNCH_PID" 2>/dev/null; do sleep 2; done
    "$WINESERVER" -k 2>/dev/null
    sleep 2
    kill_wine_procs
) &

# exec so the game runs AS this .app's process: one Dock tile, our icon,
# and winemac can activate/front the window. The absolute unix path
# matters — a bare or DOS-style name goes through start.exe.
cd "$PREFIX/drive_c/$GAME_DIR"
exec nice -n 19 "$WINE" "$PREFIX/drive_c/$GAME_DIR/"%s
`, profile.Exe)
}

// generateInfoPlist returns the Info.plist XML content for the .app bundle.
//
// Win16 apps hide the launcher from the Dock (LSUIElement) because the
// visible process is a separate Wine child. Win32 apps are regular apps:
// the launcher execs into the game, so the bundle's own Dock tile IS the
// game (see generateLauncherScript).
func generateInfoPlist(profile GameProfile) string {
	uiElement := "false"
	if profile.Win16 {
		uiElement = "true"
	}
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
    <%s/>
</dict>
</plist>
`, profile.Name, profile.Name, profile.BundleID, uiElement)
}
