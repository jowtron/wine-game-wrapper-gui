# wine-game-wrapper-gui

A native macOS GUI frontend for [wine-game-wrapper](../wine-game-wrapper/). Built with [Wails v2](https://wails.io/) (Go backend + vanilla HTML/CSS/JS frontend).

Provides the same 7-step build pipeline as the CLI tool, with a visual interface including game profile dropdown, native file pickers, a progress bar, and a scrollable build log.

## Prerequisites

Same as the CLI tool:

```bash
brew install bchunk flac
```

Plus the Wails CLI for building:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

## Development

```bash
cd wine-game-wrapper-gui
wails dev
```

This starts the app in dev mode with hot reload for frontend changes.

## Production Build

```bash
wails build
```

Produces a native `.app` bundle in `build/bin/`.

## Usage

1. Select a game profile from the dropdown (or choose "Custom" and enter an exe name)
2. Click **Browse** to select your `.cue` file
3. Optionally set an output path and advanced options (Wine path, otvdm path, etc.)
4. Click **Build .app**
5. Watch progress in the log area

The build runs the same pipeline as the CLI version — extracting the disc image, converting audio, setting up Wine, and assembling a self-contained `.app`.

## Project Structure

```
wine-game-wrapper-gui/
  main.go              # Wails entry point, embeds frontend/
  app.go               # App struct with bound methods (file dialogs, build trigger)
  pipeline.go          # 7-step build orchestrator
  progress.go          # ProgressReporter interface + Wails event emitter
  game_profiles.go     # Built-in game profiles
  extractor.go         # CUE/BIN extraction + FLAC conversion
  wine_download.go     # Wine auto-detection + download
  prefix.go            # Wine prefix setup + component installation
  builder.go           # .app bundle assembly
  launcher.go          # Launcher script + Info.plist generation
  resources/
    mcicda.dll         # CD audio stub DLL
  frontend/
    index.html         # Single-page UI
    style.css          # Dark theme (Catppuccin Mocha)
    app.js             # Frontend logic
  wails.json           # Wails config (no npm, vanilla JS)
```

## Notes

### CPU usage with Win16 games

Win16 games often use a busy-wait message loop (`PeekMessage` in a tight loop), which can consume 100%+ CPU under Wine/otvdm. Two mitigations are applied:

- **`PeekMessageSleep=1`** (in `otvdm.ini`): Adds a 1ms sleep per `PeekMessage16` call inside otvdm. This is the effective fix — it dramatically reduces CPU usage with no noticeable impact on game responsiveness. The ini file is generated automatically during the build when Win16/otvdm is enabled.

- **`nice -n 19`** (in the launcher script): Lowers the game's scheduling priority to the minimum. This doesn't reduce actual CPU usage but prevents the game from starving other processes. Could be removed if `PeekMessageSleep` proves sufficient long-term.
