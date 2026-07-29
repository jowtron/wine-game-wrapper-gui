# Game patch overlays

Overlay patch sets referenced by profiles live here, EXCEPT copyrighted game
binaries, which are gitignored. Those are resolved at build time by
`external_patches.go`:

1. `~/Library/Application Support/wine-game-wrapper/build-inputs/patches/<name>/`
   (supply your own copy — you legally own the game), then
2. a download from a known URL, cached under `~/.cache/wine-game-wrapper`.

`civnet` (Sid Meier's CivNet v1.0.2 Patch 3) is handled this way.
