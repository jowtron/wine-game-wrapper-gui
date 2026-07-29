# Wine Game Wrapper

Turn a classic Windows CD game into a **self-contained macOS `.app` you
double-click to play** — Wine, a configured prefix, your game files, and the
original **CD soundtrack** all baked into one bundle. No Homebrew Wine, no
`winecfg`, no terminal. Built and tested on Apple Silicon.

It began as a way to play *Sid Meier's Civilization II* on a modern Mac and grew
into a small pipeline that handles the awkward parts of shipping a 1990s Windows
game as a native-feeling Mac app: splitting CUE/BIN discs, ripping and replaying
Red Book **CD audio**, remapping the keyboard so **numpad-only controls work on
a laptop**, passing CD checks, theming the Windows chrome to match macOS, and
keeping your saves when you rebuild.

The GUI is [Wails v2](https://wails.io/) (Go backend, vanilla HTML/CSS/JS), with
a headless `build` subcommand for scripting.

---

## What you get in each app

- **One double-click bundle.** Wine, the prefix, and the game all live inside
  the `.app`. Copy it to another Mac and it just runs.
- **The real CD soundtrack.** The music from the original disc plays in-game —
  see [CD audio](#cd-audio) below.
- **A keyboard remapper.** Play numpad-driven games (unit movement in Civ-style
  games) on a laptop with no numpad — see [Keyboard remapper](#keyboard-remapper).
- **Light/dark Windows theming** that follows the macOS appearance.
- **Saves that survive rebuilds** (stored outside the bundle).

## Games with built-in profiles

You supply the game (see [Bring your own game files](#bring-your-own-game-files)).

| Game | Type | Notes |
|---|---|---|
| Civilization II — Multiplayer Gold | Win32 | CD music, CD-check, movies dropped (Indeo) |
| Civilization II — Test of Time | Win32 | v1.1 + [TOTPP] auto-applied; CD music; runs at high resolution |
| Sid Meier's CivNet | Win16 (otvdm) | CD music; official 1.0.2 patch fetched at build time |
| Sid Meier's Colonization | Win16 (otvdm) | CD music |
| Sid Meier's Alpha Centauri | Win32 | Renders via bundled [cnc-ddraw]; built from a DRM-free (e.g. GOG) install |

Adding another game is usually just a [TOML profile](#adding-a-game) — no code.

[TOTPP]: https://forums.civfanatics.com/threads/the-test-of-time-patch-project.517282/
[cnc-ddraw]: https://github.com/FunkyFr3sh/cnc-ddraw

---

## Bring your own game files

This is a **build tool, not a game distributor.** It never ships game
executables, data, or music. You point it at a disc image (`.cue`/`.bin`) or an
install folder you already own, and it builds an app around it. Copyrighted
patches that can't be bundled (e.g. the CivNet 1.0.2 patch) are resolved from a
copy you supply or downloaded at build time — see
[`resources/patches/README.md`](resources/patches/README.md).

---

## Quick start

Install [Wails](https://wails.io/) and (for InstallShield-based discs like Civ2)
`unshield`:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
brew install unshield          # only for extract-installshield games
```

### GUI

```bash
./build.sh gui                 # -> build/bin/Wine Game Wrapper.app
```

1. Pick a game profile (or "Custom" + an exe name).
2. Browse to your `.cue` — or, for folder-based games, a **Game Folder**.
3. Build, watch the log. The finished app lands in `/Applications`.

### Headless / scripting

```bash
wine-game-wrapper-gui build -list
wine-game-wrapper-gui build -game civ2    -cue Civ2.cue
wine-game-wrapper-gui build -game smac    -src "/path/to/GOG/Alpha Centauri"
wine-game-wrapper-gui build -game civnet  -cue CIVNET.cue -o /tmp/CivNet.app -overwrite
```

`build.sh` also rebuilds the bundled games from prepared masters — see it and
`TODO-civ2tot.md` for the reproducible-build setup.

---

## CD audio

Many of these games play their soundtrack as **Red Book CD audio** — real audio
tracks on the disc, not files the game reads. On original hardware the game asks
the OS to play "track 3" and the CD drive does it. That doesn't work from a disc
*image* on macOS, so the music normally just… doesn't play.

This project fixes it with a small **drop-in `mcicda.dll`** (built from source in
[`mcicda/`](mcicda), links libogg + libopus). At build time the pipeline:

1. Splits your `.cue`/`.bin` into the data track + the audio tracks (pure Go).
2. Encodes the audio tracks to FLAC (pure Go, parallel).
3. Installs them as `C:\music\trackNN.flac` in the prefix.

At runtime `mcicda.dll` intercepts the game's MCI `cdaudio` commands and plays
the matching track through Wine's audio (CoreAudio). Track numbering matches the
original disc (track 1 is the data track, so music starts at `track02`). The DLL
also accepts `.wav/.mp3/.ogg/.opus`.

Result: the original soundtrack plays in-game, straight from the app, with no CD
in the drive. Four of the five built-in games use it (Alpha Centauri's disc has
no CD audio).

---

## Keyboard remapper

Civ-style games move units with the **numpad** (diagonals on 7/9/1/3). Laptops
don't have a numpad, so those moves are impossible without a workaround.

The remapper (`keyhook.exe` + `keyremap.dll`, built from source in
[`keyremap/`](keyremap)) installs a global Windows keyboard hook that rewrites
keystrokes *inside* the game. A profile declares the mapping by key name:

```toml
[[keymap]]
from = "7"    # letters, digits, num0-num9
to   = "num7"
[[keymap]]
from = "u"
to   = "num4"
```

The built-in games map `7 8 9 / u o / j k l` to the numpad cluster, so the
right-hand keys drive units in all eight directions — no numpad required. Each
key resolves to its scancode + virtual-key pair, written to `keyremap.ini`, and
the hook is started automatically at launch. Leave `keymap` out and no hook runs.

---

## How a build works

1. **Wine** — cached download of a Gcenx `wine-stable` build (or a local path).
2. **Disc** — split CUE/BIN into a data ISO + WAV audio tracks (pure Go). Or,
   for folder-based games, use the install folder directly.
3. **Prefix** — `wineboot`, a virtual desktop, drive mappings, Windows version.
4. **Components** — `mcicda.dll`, otvdm (Win16), the keyremap hook.
5. **Game** — install per the profile's strategy; apply overlay/hex patches.
6. **CD audio** — install FLAC tracks into `C:\music` (see [above](#cd-audio)).
7. **Bundle** — assemble the `.app`: Wine + prefix + launcher + icon.

## Adding a game

Games are declarative TOML — drop a profile into
`~/Library/Application Support/wine-game-wrapper/profiles/`, no rebuild needed:

```toml
name = "CivNet"
exe  = "CIVNET.EXE"
win16 = true                    # run under otvdm (Win16 layer)
game_dir = "CivNet"
install = "copy-cd-root"        # copy-cd-root | copy-dir:<p> | extract-installshield |
                                #   run-installer:<p> | copy-source-dir
desktop = "1920x1080"
theme = "auto"                  # light | dark | auto
retain_cd = ["Civ2/VIDEO"]      # CD paths to bundle + map as drive d:
cd_label = "Civ2:MGE v1.0"      # MUST match the real disc label (read from the ISO PVD)
dll_overrides = "ddraw=n,b"     # extra WINEDLLOVERRIDES (e.g. load bundled cnc-ddraw)

[[overlay]]                     # copy a patch file set over the game dir
source = "civnet"               # embedded, or build-inputs/patches/civnet, or downloaded

[[hexpatch]]                    # verified in-place byte patch
file = "civnet.exe"
expect_size = 2073600
offset = 0x147cff
expect  = [0x18, 0x04, 0x68, 0x14, 0x05]
replace = [0x00, 0x7d, 0x68, 0x00, 0x7d]
optional = true

[[keymap]]
from = "7"
to   = "num7"
```

## Building from source

```bash
./build.sh gui        # GUI app (wails build) -> build/bin/
./build.sh games      # rebuild the bundled game apps from masters
./build.sh all        # both (default)
wails dev             # GUI with hot reload
```

## Roadmap

- **Intel Macs** — supported. The bundled Wine is x86_64, so the game apps run
  natively on Intel (and via Rosetta 2 on Apple Silicon). The GUI ships as a
  **universal** binary (`./build.sh gui` → `wails build -platform darwin/universal`).
- **Linux** — planned, as its own effort. The Go core (CUE/BIN splitting, FLAC,
  unshield, profiles, the `mcicda`/keyremap/cnc-ddraw payloads, the game
  knowledge) is portable; what's macOS-specific is the packaging (`.app`),
  Wine source, launcher, icons, and theme detection. Those would move behind a
  small platform interface with a Linux backend that outputs an AppImage.
  A nice bonus: the macOS rendering workarounds (cnc-ddraw, virtual-desktop
  hacks) are winemac-specific — on Linux these old games largely "just render."
  Display target: **X11 / XWayland** first — `winex11.drv` is the battle-tested
  reference path for 1990s DirectDraw/Win16 games and runs fine on Wayland
  desktops via XWayland. Native `winewayland.drv` is maturing quickly and is the
  eventual target, but X11 is the pragmatic starting point for this era of game.

## License

This project's own code is **MIT** (see [`LICENSE`](LICENSE)). It bundles or
downloads third-party components under their own licenses — Wine and winevdm
(LGPL-2.1), libogg/libopus (BSD), cnc-ddraw (GPL) — see
[`THIRD-PARTY-NOTICES.md`](THIRD-PARTY-NOTICES.md). Game data is never included
and is yours to supply.
