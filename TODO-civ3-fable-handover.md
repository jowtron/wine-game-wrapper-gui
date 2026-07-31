# Civ3 Complete — port notes (winemac). Init crash SOLVED; audio bug OPEN.

## ⚠️ OPEN 2026-07-31 — the audio system dies mid-game (music must stay off)

**Symptom chain, all user-confirmed:** with `Music Volume` > 0, ALL sound
(music *and* SFX) works for a few minutes, then dies together, leaving the last
buffer looping ("stuck record"). The game stays fully playable. Quitting then
hangs forever, spinning at ~100% CPU, and needs a force kill
(`~/Quit Civ3.command` on this Mac). With music off, none of it happens.

**REPRO TRIGGER (user, 2026-07-31) — the key to fixing this:** it dies right
around *founding the first city* → the city dialog opens → changing production
to Settlers. That is a window create/destroy moment, which fits the leading
theory below. There is a `Conquests Autosave 4000 BC.SAV` (turn 1, city not yet
founded) in the user's save dir, and Civ3 registers `.SAV` for double-click, so
passing a save path as argv[1] very likely boots straight into that state — a
3-keystroke repro instead of an hour of play.

**Leading theory (unproven):** `sound.dll` caches a notification HWND (per
stream at `[esi+0x1c]`, plus a global at `[0x100b416c]`) and posts msg `0x7F4`
to it. If that window is destroyed when the city screen opens/closes, every
later post fails, and each of the 4 post sites **retries forever**
(`"PostMesage Fail!"` → `Sleep(100)` → repost). That wedges the audio thread,
which explains all four symptoms at once — including the hung exit, where
shutdown waits on that thread. Note the game's `PostMesage Fail!` printf is
**buffered and lost on kill -9**, so an empty log proves nothing.

**Next step:** filtered relay tracing — set `RelayInclude` under
`HKCU\Software\Wine\Debug` to just `PostMessageA;timeSetEvent;waveOutWrite`
(full `+relay` is too heavy and this bug is timing-sensitive), reproduce via the
autosave, and read the first `PostMessageA` that returns FALSE plus the window
lifecycle around it. `winedbg` is plan B only: 32-bit code under new-wow64 +
Rosetta is where its breakpoint support is weakest.

**If the stale-HWND theory holds, the fix is probably 2 bytes.** The earlier
attempt (commit f555e79, reverted by d88cfcf) NOP'd the retry `je`s at file
offsets 177682 / 177964 / 178156 / 178979 (`expect_size` 454656) — that stopped
the deadlock but fell through into the *teardown* epilogue, which kills the
timer and zeroes the stream handles, so all audio went silent and the exit
still hung. The better patch jumps to the **success** path instead: drop one
notification, keep the sound system alive. Same bytes, different target.

**Ruled out, do not repeat (all tried 2026-07-31):**
1. A newer `Mss32.dll` — all 18 copies on the machine are identical Miles 6.1a,
   there is no legitimate source, and it is the wrong layer anyway: the failing
   call is `PostMessageA` inside Firaxis's own `sound.dll` (exports are
   `Dll_Wave_Device` / `WaveInDeviceMgr` / `SNDERR`, i.e. not Miles). GOG ships
   the final 1.22 patch, so there is no newer `sound.dll` either.
2. `WINEDLLOVERRIDES=…;dsound=d` to force Miles onto waveOut — `dsound.dll`
   never loads, but Miles does not fall back: no `.mp3` is opened at all.
3. WAV content under an `.mp3` filename to bypass `Mp3dec.asi` — Miles does not
   sniff the header; instant page fault at `0x26F01951`.

**Diagnostics that actually worked:** `lsof -p <pid>` for open audio files (an
absent `.mp3` = the stream died) and `sample <pid>` on the LIVE symptom.
Process-hygiene trap: a dead Civ3 process can linger for many minutes with one
thread in `__sigsuspend`, so `pgrep | head -1` can hand you a corpse — always
check `ps ax -o pid,etime` and take the process whose age matches the session.

---

## ✅ SOLVED 2026-07-30 (Fable) — init crash root-caused and fixed

The `0x5cdbe6` crash was **not** a Wine bug and not a missing constructor: the
`0xcadc0c` global is the 866-entry **labels.txt string array** (fields +0x1c/
+0x20 of the shared text-parser object at `0xcadbf0`). The crash is the game's
**"Unable to find file" popup** (jackal.txt error system) dereferencing the
not-yet-loaded labels array: init at `0x550760` resolves `text\version.txt`
BEFORE calling LoadLabels (`0x5506a0`), and the Conquests folder has no
version.txt → popup → labels NULL → fault.

Root disease: **Conquests' folder is not self-contained.** Its path resolver
(RE'd at `0x54ca10`/`0x54ca70`) searches three roots: CWD, then
`HKLM\Software\Infogrames\Civ3PTW\Install_Path`, then
`HKLM\Software\Infogrames Interactive\Civilization III\Install_Path`.
`version.txt`, `art\PopUpandMenuBackground.pcx` etc. exist only in the base
tree. The two "ruled out" attempts each supplied HALF the fix — registry keys
pointing at a dir with no base files, or the full tree with no registry keys.
**Both together work**: ship the whole GOG `app/` tree, run the exe from
`Conquests\`, seed both Install_Path values. (`Min_Install` must stay absent —
a nonzero value triggers a CD hunt for `X:\Civ3Cqst.ico` at `0x54c950`.)

Shipped changes: recursive overlays (patches.go), subdir exe support
(launcher.go cd's into the exe's dir; install.go path-aware check), profile
`[[registry]]` REG_SZ seeding (prefix.go), civ3.toml now builds from the app/
ROOT with exe `Conquests/Civ3Conquests.exe`, four Install_Path entries, and an
overlay shipping `Conquests/conquests.ini` + `Conquests/Text/version.txt`
(self-authored `#LANGUAGE 0`). Verified: menu art + cursor + script.txt load,
0 page faults, 0 PostMesage fails, process active 4+ min via the real .app.
Awaiting the user's eyes for rendering; next likely wall in-game is black
terrain (WineHQ 41930 → C3X mod). The original handover follows for history.

---

*Written 2026-07-30 by Opus after a long diagnostic session. Hand this whole file
to Fable as the task. It is self-contained — assume no memory of the session.*

---

## Your job

Get **Sid Meier's Civilization III: Complete** (GOG release) to launch, render,
and play as a macOS `.app` built by this repo's wrapper, the way the other 5
games (Civ2 MGE, Civ2 ToT, CivNet, Colonization, SMAC) already do. It is the
last, un-started stretch game. Everything up to the game's own init is already
working; there is **one hard crash** left that config can't touch. Read the
whole file before acting — a lot has been ruled out, don't re-run dead ends.

The two deliberate trade-offs already decided (keep them unless you find better):
**music off** (Miles music halts-then-hangs under Wine) and **no numpad keymap**
(its keyhook re-triggers the audio hang — see below).

---

## Project orientation (how the wrapper works)

- Repo: `/Users/joseph/Claude_Code/wine_project/wine-game-wrapper-gui/`
  (`github.com/jowtron/wine-game-wrapper-gui`). Go + Wails GUI, plus a headless
  `build` subcommand. Bundles a Wine build into each `.app`.
- Profiles are TOML in `resources/profiles/<slug>.toml`; icons in
  `resources/icons/<slug>.icns`; declarative file overlays in
  `resources/patches/<source>/`. **Resources embed at Go COMPILE time**
  (`//go:embed resources`) — after editing any `resources/` file you MUST
  `/opt/homebrew/bin/go build -o wine-game-wrapper-gui .` before the build
  subcommand sees it.
- Go is at `/opt/homebrew/bin/go` (not on PATH here).
- Headless build (works with plain `go build`):
  ```
  cd wine-game-wrapper-gui
  /opt/homebrew/bin/go build -o wine-game-wrapper-gui .
  ./wine-game-wrapper-gui build -game civ3 \
    -src "$HOME/Library/Application Support/wine-game-wrapper/build-inputs/masters/civ3/app/Conquests" \
    -wine "/Applications/Alpha Centauri.app/Contents/Resources/wine" -overwrite
  ```
  (`-wine` reuses an already-bundled Wine so it doesn't re-download ~1 GB.)
- **Build inputs live OUTSIDE the repo** at
  `~/Library/Application Support/wine-game-wrapper/build-inputs/`. Game data is
  never committed.
- **The bundled Wine is `wine-11.0`** (from the SMAC app's Resources/wine).
  This is *newer* than the CrossOver 25.1 (Wine 10.x) the public Civ3 recipe was
  tested on — so "use a newer Wine" is not the answer.
- `.app` runtime gotcha: launcher `exec`s into wine; a watchdog subshell kills
  Wine when the launch PID dies. CWD inside the game = the game dir. Saves live
  in an external seeded copy at `~/Library/Application Support/wine-game-wrapper/
  <slug>/<GameDir>` (symlinked into the prefix), seeded once from the bundle
  master on first run — so **editing files in the bundle does NOT reach an
  already-seeded install**; delete `~/Library/Application Support/
  wine-game-wrapper/civ3/` to force a clean re-seed.

## The game media (already extracted, DRM-free)

- GOG offline installer was `innoextract`ed to
  `~/Library/Application Support/wine-game-wrapper/build-inputs/masters/civ3/app/`.
- **Only ONE playable engine exists: `app/Conquests/Civ3Conquests.exe`** (3.4 MB,
  SafeDisc-free — GOG stripped it). There is no separate base-game or Play-the-
  World *game* exe in the GOG package (only their world-builder editors). Base +
  PTW rules/scenarios are all reachable from inside Conquests' own menus, so
  Conquests IS "Complete". `app/Conquests/` is self-contained (own Art/Sounds/
  Text/Scenarios/DLLs). The current profile builds from `app/Conquests` as
  `-src`; the whole `app/` tree is also present if you want it.
- Imports: **OpenGL32 + GDI32 + WinMM + binkw32 (Bink video) + Mss32 (Miles
  audio)** — NOT DirectDraw (unlike the older Civs). Music is GOG MP3s in
  `Sounds/`, played by Miles via `redist/win32/Mp3dec.asi` (no CD audio).

## Current profile state (what's already committed to the build)

`resources/profiles/civ3.toml`: `install="copy-source-dir"`, `exe=Civ3Conquests.exe`,
`game_dir=Civ3`, `theme=auto`, **`desktop="none"`**, an `[[overlay]] source="civ3"`,
and **no keymap**. `resources/patches/civ3/conquests.ini` overlays:
```
[Conquests]
KeepRes=1
PlayIntro=0
Music Volume=0
SFX Volume=128
```
Engine change made this session: `prefix.go` now skips the Wine virtual-desktop
registry keys when `desktop == "none"` (backward-compatible; the other 5 profiles
set a real size and are unaffected).

---

## The three failure layers

### Layer 1 — vdesktop → Miles PostMessage hang  (FIXED)
Inside a Wine virtual desktop, Civ3's Miles audio streaming thread `PostMessage()`s
to a window Wine then rejects; the game prints its own `"PostMesage Fail!"` (sic)
hundreds of times and hangs. Removing the virtual desktop (`desktop="none"`) +
`KeepRes=1` (no display-mode change, borderless at current res) makes the window
valid → 0–4 fails, no hang. Confirmed by direct runs.

### Layer 2 — keymap keyhook → same hang  (FIXED by removing the keymap)
The 789/uo/jkl→numpad remap injects a global low-level keyboard hook
(`keyhook.exe`). With it running, `PostMessage` fails again (~196/run vs ~4
without). So **Civ3 cannot have the keymap** — hard incompatibility, not a
preference. Removed.

### Layer 3 — init crash: null singleton  (THE BLOCKER — unsolved)
Even with layers 1–2 fixed, an **unhandled access violation** during init:
```
wine: Unhandled page fault on read access to 00000B9C at address 005CDBE6 (thread 0024)
```
Disassembly at the fault (ImageBase 0x400000, so this is game code):
```
0x5cdbd6: mov  eax, dword ptr [0xcadc0c]   ; load global singleton pointer -> it is NULL
0x5cdbdb: mov  byte ptr [0xcab2d8], bl
0x5cdbe1: mov  ecx, 0xcadbf0
0x5cdbe6: mov  eax, dword ptr [eax + 0xb9c]  ; FAULT: NULL->+0xB9C  (reads 0x00000B9C)
0x5cdbec: push eax
0x5cdbed: call 0x60f6a0                       ; then strcpy's that string into buffer 0xcab2d8
```
Register state at fault: `eax=0 ebx=0 ecx=0x00cadbf0 edx=0x01561598
esi=0x009c393f edi=0x009c3c5c ebp=0x009c3c4b`; `info[0]=0 (read) info[1]=0xB9C`.
The global at **`0xcadc0c` is the game's central singleton** — it is read in
**890** places across `.text`, so basically every subsystem uses it. It is NULL
here, i.e. it was **never constructed** during startup on our vanilla Wine 11.0.
The faulting code is reading a string field (`+0xB9C`) from it and copying that
string into a global buffer at `0xcab2d8` (looks like building a path/name).
`winedbg --auto` then attaches, which is what leaves a dead process open at 0%
CPU (this fooled early "it's alive at the menu" checks — a crashed+suspended
process looks identical to an idle menu via `pgrep`; only CPU%/the winedbg
attach reveals it).

**Ruled out for Layer 3 (do not re-try these):**
- Seeding `Install_Path` under `HKLM\Software\WOW6432Node\Infogrames Interactive\
  Civilization III` (+ `\Conquests`, + `Infogrames\Civ3PTW`) — no effect.
- Shipping the FULL `app/` tree and running the exe from the `Conquests\` subdir
  with the base game one level up — crash unchanged (not a missing-parent-files
  issue).
- Windows version `winxp` (still crashes) and `win98` (broke the launch).
- Newer Wine — already on 11.0, newer than the tested CrossOver 25.1.

---

## The recipe that IS confirmed good (for once init is fixed)

CrossOver first-party, macOS-Sequoia-tested (see the deep-research report below):
no virtual desktop; `conquests.ini` `[Conquests]` with `KeepRes=1` (borderless at
desktop res, stops the launch resolution-switch self-exit) + `PlayIntro=0` (skip
the intro Bink, which otherwise blocks the menu). Music off / SFX on because
music halts-then-hangs under Wine (Miles timer bug) — no source found any config
that keeps music working; the "IndirectSound dsound.dll" idea was explicitly
refuted. **Likely NEXT wall after init: "black terrain"** (WineHQ bug 41930) —
Civ3 renders OpenGL v1.x into offscreen DIBs and shows black terrain; on macOS
the OSMesa-removal trick does NOT apply (that's Linux/Mesa), the community's
answer is the C3X exe mod's GDI+ path (below).

## Leads for Layer 3 (starting points — you may well find better)

These are ideas, not a prescription. The core question is *why the `0xcadc0c`
singleton is never constructed*.

- **Find the constructor and the swallowed exception.** ~11 first-chance
  `c0000005` exceptions fire during init *before* the fatal one and are
  SEH-handled; one of them may be the singleton's constructor failing and being
  swallowed, leaving the global NULL. Locate the write to `0xcadc0c` (byte
  pattern `A3 0C DC CA 00` for `mov [0xcadc0c],eax`, or `89 xx` forms) and trace
  what runs it / what it depends on. `WINEDEBUG=+seh,+relay` or a winedbg script
  with a breakpoint on the constructor would pin the exact failing call. RE tools
  used this session: a scratch python venv with `pefile` + `capstone` (32-bit).
- **The GOG installer registers things our bare prefix lacks.** We build via
  `innoextract` + copy-source-dir, which skips whatever the real `setup.exe`
  does (COM/DirectX registration, VC/DirectX redist, extra registry). The
  singleton reads a string (path/name) then deref-crashes — plausibly a
  COM/registry/DirectX dependency. Worth trying: run the actual GOG installer
  under Wine into a throwaway prefix and diff its registry/system32 against ours;
  or `winetricks` the likely runtimes (directx/vcrun) into the prefix.
- **CrossOver/Whisky reportedly reach the menu.** CrossOver ships proprietary
  per-app Wine patches; the CivFanatics "Installing, Playing and Modding C3C on
  Apple Silicon" thread has people running C3C on Apple-Silicon Macs. Confirming
  whether stock Whisky (Gcenx Wine, like ours) hits this *same* `0x5cdbe6` crash
  would tell you if the blocker is the Wine build vs. our install method. If
  Whisky gets past it, diff its prefix/setup against ours.
- **C3X mod** (`github.com/maxpetul/C3X`) — the Apple-Silicon community's Civ3
  solution; its `draw_lines_using_gdi_plus = wine` replaces Civ3's OpenGL bitmap
  drawing with GDI+ (the macOS-robust fix for the black-terrain wall). It patches
  the exe and may shift init behavior too — but it targets *rendering*, so it may
  not by itself fix this earlier init crash. Likely still needed for the terrain
  once init works.

## Reference: the deep-research report

A full fan-out + adversarially-verified research report on Civ3-on-Wine/macOS
(the source of the confirmed recipe and the C3X / OSMesa / audio findings) is
saved durably at:
`~/Library/Application Support/wine-game-wrapper/build-inputs/masters/civ3/deep-research-report.json`
Key primary sources: CodeWeavers CrossOver tip page for Civ3 Complete on macOS;
CivFanatics C3C-on-Apple-Silicon thread; WineHQ bug 41930 (black terrain);
PCGamingWiki Civilization III; paulthetall / Porting Kit Mac guide.

## Reproduce the crash fast (direct run, bypasses the .app launcher)

```
APP="/Applications/Civilization III Complete.app"
PREFIX="$APP/Contents/Resources/wineprefix"; WINE="$APP/Contents/Resources/wine/bin/wine"
cd "$PREFIX/drive_c/Civ3"          # (build the app first; this is the seeded game dir symlink)
export WINEPREFIX="$PREFIX" WINEARCH=wow64 WINEDEBUG="+seh"
export WINEDLLOVERRIDES="mcicda=n;keyremap=n;mscoree=d;mshtml=d"
export DYLD_FALLBACK_LIBRARY_PATH="$APP/Contents/Resources/wine/lib"
"$WINE" "$PREFIX/drive_c/Civ3/Civ3Conquests.exe" 2>&1 | grep -iE "Unhandled|005cdbe6|PostMesage"
# cleanup: "$WINE" wineserver -k ; pkill -9 -f "$APP/Contents/Resources/wine"
```
A hard rule from this project: **only the user's eyes confirm rendering** — a
crashed/frozen/black/grey window all look "alive" headlessly. Have Joseph look.
```
