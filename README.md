# wine-game-wrapper-gui

Build self-contained macOS `.app` bundles from CUE/BIN CD images of classic Windows games. The resulting `.app` includes Wine, a configured prefix, game files, and CD audio — everything needed to double-click and play.

Native macOS GUI built with [Wails v2](https://wails.io/) (Go backend + vanilla HTML/CSS/JS frontend), plus a headless `build` subcommand for scripting.

## Prerequisites

None for most games — CUE/BIN splitting and FLAC encoding are pure Go, and `hdiutil`/`cp` ship with macOS.

- Games that use an InstallShield 5 CD (e.g. Civ2 Multiplayer Gold): `brew install unshield`
- Building the app itself: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

## Game profiles

Games are described by declarative TOML profiles — no code changes needed to add a game. Builtins live in `resources/profiles/*.toml`; drop additional profiles into:

```
~/Library/Application Support/wine-game-wrapper/profiles/
```

A profile:

```toml
name = "CivNet"
exe = "CIVNET.EXE"
win16 = true                     # needs otvdm (Win16 compatibility layer)
game_dir = "CivNet"              # directory under C:\
install = "copy-cd-root"         # how files get from the CD into the prefix
desktop = "1920x1080"            # Wine virtual desktop size
retain_cd = ["Civ2/VIDEO"]       # CD paths to bundle and map as drive d:
cd_label = "CIV2_MGE"            # volume label for the emulated d:

[[overlay]]                      # copy a patch file set over the game dir
source = "civnet"                # -> resources/patches/civnet/
skip = ["patch.txt"]

[[hexpatch]]                     # verified in-place byte patch
file = "civnet.exe"
desc = "widescreen fix"
expect_size = 2073600            # exact size guard (0 = skip check)
offset = 0x147cff
expect = [0x18, 0x04, 0x68, 0x14, 0x05]
replace = [0x00, 0x7d, 0x68, 0x00, 0x7d]
optional = true                  # warn-and-continue instead of failing

[[keymap]]                       # keyboard remapping by key name
from = "7"                       # letters, digits, num0-num9
to = "num7"
```

### Install strategies

| `install =` | Behavior |
|---|---|
| `copy-cd-root` | Copy the whole data track into `C:\<game_dir>` (run-from-CD games) |
| `copy-dir:<path>` | Copy one directory from the CD |
| `extract-installshield` | Extract `data1.cab` with unshield (`:<dir>` if not at CD root) |
| `run-installer:<path>` | Run the CD's installer under Wine — interactive, the CD is visible as `d:` |

`retain_cd` bundles CD directories into the .app (`Resources/cdrom`) and maps them as a `d:` CD-ROM drive (registry `Type=cdrom` + volume label), for games that pass CD checks or stream videos/music from the CD at runtime.

## GUI usage

```bash
wails dev      # development with hot reload
wails build    # production .app in build/bin/
./build-app.command   # universal (arm64 + amd64) build
```

1. Select a game profile from the dropdown (or "Custom" and enter an exe name)
2. Browse to your `.cue` file
3. Optionally set output path / advanced options
4. Build, watch the log

## Headless usage

```bash
wine-game-wrapper-gui build -list                       # show available profiles
wine-game-wrapper-gui build -game civnet -cue CIVNET.cue
wine-game-wrapper-gui build -game civnet -cue CIVNET.cue -o /tmp/CivNet.app -overwrite
wine-game-wrapper-gui build -exe GAME.EXE -win16 -cue game.cue   # custom game
```

## Project structure

```
wine-game-wrapper-gui/
  main.go              # Wails entry point + headless `build` subcommand
  app.go               # Bound methods for the frontend (dialogs, build trigger)
  pipeline.go          # 7-step build orchestrator
  profiles.go          # TOML profile loading, key table, registry
  install.go           # Install strategies + retained-CD staging
  patches.go           # Declarative overlay + hex patch application
  progress.go          # ProgressReporter: Wails events / stdout
  cuebin.go            # Pure-Go CUE/BIN track splitting
  flac_encode.go       # Pure-Go WAV->FLAC (parallel)
  extractor.go         # CUE parsing, data track discovery
  wine_download.go     # Wine auto-detection + download (Gcenx builds)
  prefix.go            # Wine prefix setup + component installation
  builder.go           # .app bundle assembly
  launcher.go          # Launcher script + Info.plist generation
  resources/
    profiles/          # Builtin game profiles (TOML)
    patches/           # Patch file sets referenced by profiles
    otvdm/             # Win16 compatibility layer (otya128/winevdm v0.9.0)
    mcicda.dll         # CD-audio DLL (github.com/jowtron/mcicda-stub)
    keyremap/          # keyhook.exe + keyremap.dll
    icons/             # Per-game .icns
  frontend/            # Single-page UI (vanilla JS, Catppuccin Mocha)
```

## Pipeline

1. Obtain Wine (cached download of Gcenx wine-stable, or local path)
2. Split CUE/BIN into data ISO + WAV audio tracks (pure Go)
3. Initialize the Wine prefix (win95 version for Win16, virtual desktop, drives)
4. Install components (mcicda.dll into system32+syswow64, otvdm, keyremap)
5. Install the game per the profile's install strategy; apply patches; stage retained CD content
6. Convert audio to FLAC (parallel, pure Go) into `C:\music`
7. Assemble the .app (Wine + prefix + launcher + icon + cdrom)

## Notes

### CPU usage with Win16 games

Win16 games often use a busy-wait message loop (`PeekMessage` in a tight loop), which can consume 100%+ CPU under Wine/otvdm. Two mitigations are applied:

- **`PeekMessageSleep=1`** (in `otvdm.ini`): Adds a 1ms sleep per `PeekMessage16` call inside otvdm. This is the effective fix — it dramatically reduces CPU usage with no noticeable impact on game responsiveness. The ini file is generated automatically during the build when Win16/otvdm is enabled.

- **`nice -n 19`** (in the launcher script): Lowers the game's scheduling priority to the minimum. This doesn't reduce actual CPU usage but prevents the game from starving other processes.

### CD audio

`mcicda.dll` intercepts MCI cdaudio commands and plays `C:\music\trackNN.{flac,wav,mp3,ogg,opus}` through Wine's waveOut (CoreAudio). Track numbering matches the original CD layout (track 1 = data track, so audio starts at track 2).
