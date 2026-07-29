#!/bin/bash
#
# build.sh — build the Wine Game Wrapper GUI app and/or the game .apps.
#
#   ./build.sh gui         Build the GUI app (Wails) -> build/bin/wine-game-wrapper-gui.app
#   ./build.sh smac        Rebuild Alpha Centauri.app from its master
#   ./build.sh civ2tot     Rebuild Civ2 Test of Time.app from its master
#   ./build.sh games       Rebuild both game apps
#   ./build.sh all         GUI + both game apps  (default)
#
# Game apps are built from the prepared masters under build-inputs/ (see
# TODO-civ2tot.md). Wine is taken from ~/.cache/wine-game-wrapper (downloaded
# once, then reused). Override the output dir with OUT_DIR=/some/path.
#
set -euo pipefail
cd "$(dirname "$0")"
export PATH="/opt/homebrew/bin:$HOME/go/bin:$PATH"

MASTERS="$HOME/Library/Application Support/wine-game-wrapper/build-inputs/masters"
OUT_DIR="${OUT_DIR:-/Applications}"

build_gui() {
    echo "==> Building GUI app (wails build)…"
    wails build
    echo "    -> build/bin/wine-game-wrapper-gui.app"
}

# The 'build' subcommand exits before wails.Run, so a plain 'go build' binary
# runs it fine and needs no Wails build tags. Built into build/ so it is never
# mistaken for the GUI app (double-clicking a bare Wails binary errors with
# "Wails applications will not build without the correct build tags").
CLI_BIN="build/wgw-cli"
build_cli_binary() {
    mkdir -p build
    go build -o "$CLI_BIN" .
}

# build_game <slug> <master-subpath> <app-name>
build_game() {
    local slug="$1" sub="$2" appname="$3"
    echo "==> Building $appname from master ($slug)…"
    "./$CLI_BIN" build \
        -game "$slug" \
        -src "$MASTERS/$sub" \
        -o "$OUT_DIR/$appname" \
        -overwrite
}

case "${1:-all}" in
    gui)     build_gui ;;
    smac)    build_cli_binary; build_game smac    "smac/AlphaCentauri" "Alpha Centauri.app" ;;
    civ2tot) build_cli_binary; build_game civ2tot "civ2tot/ToT"        "Civ2 Test of Time.app" ;;
    games)   build_cli_binary
             build_game smac    "smac/AlphaCentauri" "Alpha Centauri.app"
             build_game civ2tot "civ2tot/ToT"        "Civ2 Test of Time.app" ;;
    all)     build_gui
             build_cli_binary
             build_game smac    "smac/AlphaCentauri" "Alpha Centauri.app"
             build_game civ2tot "civ2tot/ToT"        "Civ2 Test of Time.app" ;;
    *)       echo "usage: $0 {gui|smac|civ2tot|games|all}"; exit 1 ;;
esac
echo "Done."
