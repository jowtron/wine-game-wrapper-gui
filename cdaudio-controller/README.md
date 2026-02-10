# cdaudio-controller

A Go companion for the [mcicda v1 stub DLL](../mcicda/). Watches `C:\mcicda_commands.log` for MCI commands and plays audio tracks using the host OS audio player.

**Note:** This is the legacy approach, superseded by mcicda v2 which plays audio in-process. Kept as a reference implementation and fallback.

## How it works

1. mcicda v1 DLL logs MCI commands (PLAY, STOP, PAUSE, RESUME) to `C:\mcicda_commands.log`
2. This controller watches the log file for new lines
3. On PLAY, launches the host audio player (`afplay` on macOS, `paplay` on Linux)
4. On STOP/PAUSE/RESUME, sends signals to the player process

## Audio formats

Supports any format the host player supports: FLAC, WAV, MP3, OGG, M4A, AAC.

Track files are expected at `C:\music\trackNN.{ext}` (e.g. `track02.flac`).

## Building

```bash
go build -o cdaudio-controller
```

Cross-compile for multiple platforms:

```bash
GOOS=darwin GOARCH=arm64 go build -o dist/cdaudio-controller-darwin-arm64
GOOS=darwin GOARCH=amd64 go build -o dist/cdaudio-controller-darwin-amd64
GOOS=linux  GOARCH=amd64 go build -o dist/cdaudio-controller-linux-amd64
GOOS=linux  GOARCH=arm64 go build -o dist/cdaudio-controller-linux-arm64
```

## Usage

```bash
# Start controller watching for commands
./cdaudio-controller \
  -log-file /path/to/wineprefix/drive_c/mcicda_commands.log \
  -music-dir /path/to/wineprefix/drive_c/music

# In another terminal, start the game with v1 DLL
WINEPREFIX=/path/to/wineprefix WINEDLLOVERRIDES="mcicda=n" wine GAME.EXE
```

The controller must be started **before** the game, or it will miss initial commands.

## Flags

| Flag | Description | Default |
|------|-------------|---------|
| `-music-dir` | Directory containing audio tracks | `./music` next to executable |
| `-log-file` | Path to mcicda_commands.log | Auto-detected from WINEPREFIX |
| `-wine-prefix` | Wine prefix path (for log file auto-detection) | `$WINEPREFIX` env var |
