#!/bin/bash
cd "$(dirname "$0")"

# Ensure Go and Wails are on PATH
export PATH="$HOME/go/bin:/usr/local/go/bin:/opt/homebrew/bin:$PATH"

# Check dependencies
if ! command -v go &>/dev/null; then
    echo "Error: Go is not installed. Run: brew install go"
    echo "Then:  go install github.com/wailsapp/wails/v2/cmd/wails@latest"
    exit 1
fi
if ! command -v wails &>/dev/null; then
    echo "Error: Wails is not installed. Run:"
    echo "  go install github.com/wailsapp/wails/v2/cmd/wails@latest"
    exit 1
fi

APP="build/bin/wine-game-wrapper-gui.app"
BIN="$APP/Contents/MacOS/wine-game-wrapper-gui"

echo "Building wine-game-wrapper-gui.app (universal)..."
echo ""

# Build arm64
echo "=== Building arm64 ==="
CGO_ENABLED=1 GOARCH=arm64 wails build -platform darwin/arm64
if [ $? -ne 0 ]; then
    echo "arm64 build failed."
    exit 1
fi
cp "$BIN" /tmp/wine-game-wrapper-gui-arm64

# Build amd64
echo ""
echo "=== Building amd64 ==="
CGO_ENABLED=1 GOARCH=amd64 wails build -platform darwin/amd64
if [ $? -ne 0 ]; then
    echo "amd64 build failed."
    exit 1
fi
cp "$BIN" /tmp/wine-game-wrapper-gui-amd64

# Combine into universal binary
echo ""
echo "=== Creating universal binary ==="
lipo -create /tmp/wine-game-wrapper-gui-arm64 /tmp/wine-game-wrapper-gui-amd64 -output "$BIN"
rm -f /tmp/wine-game-wrapper-gui-arm64 /tmp/wine-game-wrapper-gui-amd64

# Verify
echo ""
lipo -info "$BIN"
echo ""
echo "Done! App is at: $APP"
open build/bin/
