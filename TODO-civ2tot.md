# Civ2: Test of Time & Alpha Centauri — build recipe / reproducibility

Both games now BUILD, RENDER and PLAY (user-confirmed, 2026-07-29), with the
789/uo/jkl→numpad keymap enabled. This documents exactly how, so a rebuild or a
fresh machine reproduces them.

## What lives where

Prepared, working inputs are kept OUTSIDE the repo (they contain licensed game
data) under:

    ~/Library/Application Support/wine-game-wrapper/build-inputs/
      assets/            # ingredients to regenerate a master from scratch
        civ2tot11en.exe        # official ToT v1.1 patch (InstallShield SFX)
        TOTPPv018.4/           # Test of Time Patch Project v0.18.4
        cnc-ddraw-7.1.0.0.zip  # cnc-ddraw (GPL) for SMAC
      masters/
        smac/AlphaCentauri     # -src for the smac profile (GOG install + cnc-ddraw)
        civ2tot/ToT            # -src for the civ2tot profile (v1.1 + TOTPP + merged)
        civ2tot/music/         # ToT's 13 CD audio tracks, pre-ripped to FLAC.
                               # A copy-source-dir build installs a `music/`
                               # dir found beside the -src folder into C:\music
                               # (played by mcicda). Rip once from the ToT CD;
                               # this keeps CD music reproducible without the disc.

The **masters are the `-src` inputs** for the two `copy-source-dir` profiles.
Rebuild with, e.g.:

    ./wine-game-wrapper-gui -profile civ2tot \
        -src "~/Library/Application Support/wine-game-wrapper/build-inputs/masters/civ2tot/ToT"
    ./wine-game-wrapper-gui -profile smac \
        -src "~/Library/Application Support/wine-game-wrapper/build-inputs/masters/smac/AlphaCentauri"

(Confirm the exact flag names against main.go; the GUI has a folder picker.)

## Alpha Centauri (SMAC) — the master

SMAC white-screens under modern winemac because its old fullscreen DirectDraw
mode-switch can't be presented (why GOG shipped it on ancient X11 Wine). Fix:
**cnc-ddraw** (bundled in the master), loaded via `dll_overrides = "ddraw=n,b"`.

The master = a DRM-free GOG "Alpha Centauri" install PLUS, dropped into the
game dir:
  - `ddraw.dll`, `ddraw.ini`, `Shaders/` from cnc-ddraw-7.1.0.0.zip
  - a `[terranx]` section appended to ddraw.ini:

        [terranx]
        windowed=true
        maintas=true
        renderer=gdi       # ONLY gdi presents on winemac; opengl=grey, d3d9=black
        devmode=true       # don't lock the cursor

Profile: `desktop=1024x768`, `dll_overrides="ddraw=n,b"`, keymap on.
KNOWN LIMITATION: SMAC runs windowed (1024x768). True fullscreen is not
achievable — the GPU renderers don't present on winemac, and GDI upscaling
corrupts SMAC's fly-out menus/mouse mapping. Windowed is the working config.

## Test of Time (ToT) — the master

Raw InstallShield extraction of the CD does NOT work. The master adds three
things, in order:

1. **Official v1.1 patch.** Extract `civ2tot11en.exe` (7-zip → InstallShield
   cab → `unshield x data1.cab`) and overlay the new `civ2.exe` + updated
   scenario text files onto the v1.0 install (keep the v1.1 files; they are
   newer). Fixes the game version TOTPP requires.

2. **TOTPP v0.18.4.** Copy `TOTLauncher.exe`, `TOTPP.dll`, `lua.dll` (+ its
   subfolders) into the game dir, run `TOTLauncher.exe` once under this app's
   Wine, click **Patch**. This rewrites `civ2.exe` to import `totpp.dll`
   (TOTLauncher backs the original up as `civ2.bak`). TOTPP fixes the 64-bit
   menu-build crash — a `LocalAlloc(LMEM_MOVEABLE,0)`/`LocalLock`→NULL write at
   civ2.exe:0x59EEAA reached when starting a game — and disables the CD check.
   Wine's builtin `vcruntime140`/`msvcp140`/`ucrtbase` satisfy TOTPP's deps;
   the launcher disables the Mono/Gecko prompt (`mscoree=d;mshtml=d`).

3. **Merge the split game-type folders.** InstallShield splits each game type
   into two components — e.g. `Original/` (sprites) and `Original Game Files/`
   (Labels.txt + .bmp graphics). The real installer merges them; unshield does
   not. Without the merge, starting an "Original" game dies with
   **"Error -8 in module 4"** = a failed `C:\ToT\Original\LABELS.TXT` load
   (the game chdir's into the game-type folder and never falls back to root).
   Fix: `rsync -a --ignore-existing <X>_Game_Files/<X>/ <X>/` for Original,
   Fantasy, SciFi (ExtendedOriginal/Midgard already carry Labels.txt). The
   master has this done and the redundant `*_Game_Files` dirs removed.

ToT renders with **builtin DirectDraw** (no cnc-ddraw — it never had SMAC's
presentation problem). Running it in a larger virtual desktop
(`desktop=1680x1050`) lets it render at a bigger resolution (Game > Graphic
Options offers modes up to the desktop size). No CD is needed (TOTPP disables
the check), so the profile has no `retain_cd`/`cd_label`.

## Engine changes (this session)

- `profiles.go`: new `dll_overrides` field, appended to the launcher's
  `WINEDLLOVERRIDES`.
- `launcher.go`: base overrides now include `mscoree=d;mshtml=d` (kills the
  Mono/Gecko install prompt for all games); appends `profile.DLLOverrides`.
- `smac.toml`: `dll_overrides="ddraw=n,b"` + 789/uo/jkl keymap.
- `civ2tot.toml`: `install="copy-source-dir"`, `desktop="1680x1050"`,
  789/uo/jkl keymap, CD config removed.

## Not yet done

- No fresh end-to-end rebuild has been run from the masters yet (the working
  apps in /Applications are the live-patched ones). A verified rebuild is the
  next step before trusting the profiles blind.
- SMAC keymap enabled in the live prefix but not yet user-confirmed in play
  (it trades u/o/j/k/l letter hotkeys for numpad movement).
