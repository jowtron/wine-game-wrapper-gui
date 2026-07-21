# TODO: Civ2 Dock icon + window-fronting polish

Two open, Win32-specific polish problems on the Civ2 (Civilization II MGE) `.app`.
A previous session (Opus, 2026-07-21) could not solve them cleanly and caused a
regression trying — this doc captures what was learned so the next attempt starts
ahead. It doubles as a ready-to-use prompt for a fresh session (see bottom).

## The two problems

**1. The Dock icon.** When Civ2 launches, our nice icon (`resources/icons/civ2.icns`)
shows briefly, then Wine's macOS driver replaces it with the game's own ugly
16-colour icon. The 16-bit games (CivNet, Colonization) don't have this — winemac
can't read their old NE-format icon resources, so the icon set on the Wine binary
sticks; only Win32 PE icons get read and override ours.

**2. The window opens behind everything.** On launch, the game's first window and
dialogs open *behind* other windows; you must use Mission Control to surface them.
They don't come to front / grab focus on their own.

## What was already tried (don't just redo these)

- **`LSUIElement`** (in `generateInfoPlist`, launcher.go) is `true` to hide the
  launcher's Dock tile. This is the likely cause of Problem 2: an LSUIElement
  "accessory" app can't activate/front its windows. Flipping to `false` DID make
  fronting better but produced TWO Dock tiles — the bash launcher's tile AND
  Wine's separate tile — because the launcher is a shell script that spawns wine
  as a child process. Neither setting is right alone.
- **rcedit PE-icon replacement** (`wine rcedit civ2.exe --set-icon our.ico` at
  build time): mechanically worked (our multi-size icon embedded, exe still ran)
  but was a dead end for THREE reasons:
  1. It did **not** change the Dock tile — winemac appears to use the game's
     *runtime* window icon (WM_SETICON), not the file's default icon resource.
  2. The game runs `civ2.exe` from the **external save dir** (see save-redirect
     below), not the bundle copy — so bundle-exe edits never reach an existing
     install.
  3. **CRITICAL REGRESSION:** running `wine` during the build while the `d:`
     dosdevices symlink doesn't exist yet made Wine reclassify `d:` from `cdrom`
     to `floppy` in the registry, which **broke Civ2's CD-ROM check**. Fully
     reverted; build is back to known-good.

## Invariants you must NOT break

- **CD check:** Civ2 scans CD-ROM drives for a disc labelled `Civ2:MGE v1.0`. The
  build stages marker files into `Resources/cdrom`, sets that volume label, and
  sets `d:=cdrom` in `[Software\Wine\Drives]` (prefix.go `initWinePrefix`, when
  `retain_cd` is non-empty). The launcher symlinks `dosdevices/d: -> ../../cdrom`
  at runtime. **Any wine invocation at build time with no `d:` symlink present
  can clobber the drive type.** After any change, verify `d:` stays `cdrom` and
  relaunch to confirm no "can't find the CD-ROM" dialog.
- **Save redirect:** the game dir ships as a pristine master in
  `Resources/game/<GameDir>`; on first launch the launcher copies it to
  `~/Library/Application Support/wine-game-wrapper/<slug>/<GameDir>` and symlinks
  it into `drive_c`. The RUNNING game files are the external copy — changes to
  bundle game files don't reach an already-seeded install.
- The launcher relies on staying alive (bash) to run cleanup traps that kill
  wineserver on quit — any `exec`-based restructure must preserve that cleanup.

## Where the solution probably lives

The root cause of both problems is architectural: a bash launcher spawns wine as
a *child* process, so there are two processes → two Dock tiles + the child's window
can't be fronted by the accessory parent. Study how real Wine-on-Mac wrappers
(Whisky, Porting Kit, CrossOver) achieve "one Dock tile, right icon, window fronts"
— likely by making wine adopt the `.app` bundle's own identity/icon rather than a
shell launcher spawning it as a child.

These are visual/interactive bugs that can't be fully verified headlessly — plan
first, test on throwaway copies, and have the user confirm what they see before
swapping anything into `/Applications`.

---

## Ready-to-use session prompt

> This project (~/Claude_Code/wine_project) packages classic Windows games into
> self-contained macOS .app bundles that run under Wine. The maintained tool is the
> Go/Wails app in wine-game-wrapper-gui/. Read PROJECT_JOURNAL.md, then this file
> (TODO-civ2-polish.md) which has full context and the dead ends already hit, then
> form your own view from the code. I want you to solve the two Civ2 (Win32) polish
> problems described here — the Dock showing the game's ugly 16-colour icon instead
> of ours, and the game window opening behind everything instead of coming to front.
> Learn from what was already tried (rcedit is a dead end AND it broke the CD check;
> LSUIElement true/false each has a downside). Do NOT break the CD check or the save
> redirect — both invariants are documented here. Explore, propose an approach, and
> agree it with me before large changes. These are visual bugs I can verify by
> running the app and reporting what I see. Ask me anything that changes your approach.
