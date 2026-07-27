# SOLVED: Civ2 Dock icon + window-fronting polish

Both Win32-specific polish problems on the Civ2 `.app` were solved on 2026-07-21
(Fable session), after an earlier attempt (Opus, same day) hit dead ends. This doc
records the root causes and the fix so the reasoning isn't lost. Original problem
statement and the dead ends are preserved at the bottom — one of the earlier
conclusions (the WM_SETICON theory) turned out to be wrong and is corrected here.

## Root causes (from winemac.drv source, wine-11.0)

**Dock icon.** `macdrv_SetDesktopWindow` (window.c) calls `set_app_icon()`, which
reads the **first RT_GROUP_ICON resource of the process's main exe** via
`EnumResourceNamesW(NULL, RT_GROUP_ICON, ...)` (dllmain.c `macdrv_app_icon`) and
sets it as the Dock icon when the process transforms to a regular app
(cocoa_app.m `transformProcessToForeground:`). It is **not** the runtime
WM_SETICON window icon, as previously concluded — the rcedit test looked like
that only because the patched bundle exe never runs (save-redirect: the seeded
external copy runs). civ2.exe carries five RT_GROUP_ICONs, each one 32×32
16-colour image — that's the ugly icon. If no RT_GROUP_ICON is found, winemac
sets `applicationIconImage:nil` → the Dock keeps the bundle/file icon.

**Two processes, two tiles, no fronting.** The bash launcher spawned wine as a
child, so LaunchServices saw two apps: the launcher (hidden via LSUIElement, and
as an accessory app unable to front anything) and wine's own process (which
transforms to Foreground with a bare "wine" identity). Additionally, `wine
<exe>` on this Gcenx wow64-only build **always rerouted through `start.exe
/exec`**, putting the game in yet another process: ntdll's
`get_alternate_wineloader()` (loader.c) sees a 32-bit main exe and returns the
path of the i386 loader **without checking it exists** (it doesn't, in a
wow64-only build), so `load_main_exe` is abandoned (env.c). Setting
`WINEARCH=wow64` short-circuits this (`if (force_wow64) return NULL;`) and the
exe loads in-process. This is safe against the prefix: for a 64-bit prefix only
`WINEARCH=win32` errors (server.c).

## The fix (three parts, all in the generator)

1. **launcher.go** — Win32 launchers now end with
   `exec nice -n 19 "$WINE" "$PREFIX/drive_c/$GAME_DIR/<exe>"` after
   `export WINEARCH=wow64`, so the game runs *as* the .app's process:
   LaunchServices keeps the bundle identity across exec (verified:
   `lsappinfo` shows bundle path + `originalExecutablePath`) → one Dock tile,
   bundle icon/name, and winemac's `tryToActivateIgnoringOtherApps:` can front
   the window. The absolute unix path matters — a bare or DOS-style arg still
   goes through start.exe. Cleanup traps can't survive exec, so a **watchdog
   subshell** forked pre-exec waits for the PID to die, then runs
   `wineserver -k` + the kill sweep. Win16 games keep the old spawn/wait/trap
   launcher (otvdm runs in separate processes; exec gains nothing).
2. **Info.plist** — `LSUIElement` is now `false` for Win32 games (the bundle
   tile IS the game), still `true` for Win16.
3. **peicon.go** — `hidePEGroupIcon()` renames resource type 14
   (RT_GROUP_ICON → 0x0FFF) in the game exe: an in-place, reversible 4-byte
   edit, pure Go, **no wine invocation at build time** (so the d:-drive/CD-check
   regression from the rcedit attempt can't recur). Called from pipeline.go for
   Win32 games that ship our own icns. LoadIcon in-game then finds no icon
   (harmless: NULL class icon), and winemac leaves the Dock icon alone.

## Bonus find: the REAL cdrom→floppy mechanism (mountmgr drive stealing)

While verifying, the installed app's registry was found freshly broken again:
`"d:"="floppy"`, with `dosdevices/d:` rebound to `/Volumes/DOSBox Staging` (a
mounted DMG). No build had run — this happens **at runtime**. Mechanism, from
mountmgr source (device.c `add_dos_device`, unixlib.c `add_drive`):

- wineboot at **build time** records whatever CD-like device is mounted on the
  build machine as the prefix's `d::` DEVICE link (distinct from the `d:` mount
  link). That link ships in the bundle.
- At runtime, when DiskArbitration reports a volume whose device matches `X::`,
  mountmgr **reuses that letter**: rewrites the `X:` symlink to the volume's
  mount point and force-writes the drive type to the registry. For
  HARDDISK-class volumes (disk images!) Wine's type-name hack writes
  **"floppy"** — instantly breaking the CD check.
- A letter is immune iff its `X::` link is absent AND its `X:` mount link is
  present. The launcher always creates `d:`, so deleting `d::` makes d: safe.
- This also reframes the original build-time regression: during a build-time
  wine run the `d:` mount link doesn't exist yet, so the letter is "available"
  and any mounted DMG claims it (add_drive assigns CD volumes starting at d).
  Same mechanism, two entry points.

Fixes: the launcher now removes `d::` alongside the other links, and builder.go
strips all `*::` device links from the shipped prefix (they're build-machine
artifacts). The installed app's registry was repaired in place (floppy→cdrom).
Backups + the patch tool live in `~/Library/Application Support/
wine-game-wrapper/civ2/`: `civ2.exe.pre-icon-hide`, `system.reg.backup-2026-07-21`,
`hide_pe_icon.py` (`python3 hide_pe_icon.py <exe> hide|restore`).

**Outcome (user-confirmed 2026-07-21):** Dock icon correct, and CD music works
for the first time ever — the game only enables CD audio when it finds its CD
in a CD-ROM-typed drive, which the mountmgr stealing had been silently breaking
on every run. New build swapped into /Applications. Window-fronting on the new
build **user-confirmed working 2026-07-21** ("fronting works on your new civ2
build, yay!") — the earlier "doesn't front" report was against the old
architecture, which cannot front. All three polish items (icon, fronting, CD
music) are now confirmed on the live /Applications/Civ2.app.

Also fixed on the way: **process leak on quit** — Wine-spawned services
(`explorer.exe /desktop`, winedevice.exe...) show only `C:\` paths in ps and
reparent to launchd, so the old path-based pkill missed them (a stale explorer
from a morning run was found still alive). `kill_wine_procs` now also sweeps
any `[A-Z]:\`-style process whose mapped binary (lsof) lives in this bundle.

## Migration note for existing installs

The seeded live copy in `~/Library/Application Support/wine-game-wrapper/civ2/`
runs, not the bundle master — it was patched in place on 2026-07-21 (backup:
`civ2.exe.pre-icon-hide` next to the game dir). Fresh seeds inherit the patched
master from the rebuilt bundle.

## Invariants (still true, still must not break)

- **CD check:** build stages markers into `Resources/cdrom`, labels the volume
  `Civ2:MGE v1.0`, sets `d:=cdrom` in `[Software\Wine\Drives]` (prefix.go). The
  launcher symlinks `dosdevices/d:` at runtime. Any build-time wine run without
  the d: symlink can reclassify the drive and break the check. The icon patch
  is pure Go specifically to avoid this.
- **Save redirect:** game dir ships as pristine master in `Resources/game/`;
  the external seeded copy is what actually runs.

---

## Original problem statement (historical)

**1. The Dock icon.** When Civ2 launches, our nice icon shows briefly, then
Wine's macOS driver replaces it with the game's own ugly 16-colour icon. The
16-bit games don't have this — winemac can't read NE-format icon resources.

**2. The window opens behind everything;** you must use Mission Control to
surface it. (LSUIElement=true made the launcher an accessory app that can't
activate its children; flipping it to false alone produced two Dock tiles.)

**Dead ends hit by the first attempt:** rcedit --set-icon at build time (wrong
exe — save-redirect; and running wine at build time with no d: symlink
reclassified d: cdrom→floppy, breaking the CD check — fully reverted);
LSUIElement flipping alone (two tiles ↔ no fronting).
