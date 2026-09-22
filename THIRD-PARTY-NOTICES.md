# Third-party notices

This project's own code is under the MIT License (see `LICENSE`). It bundles,
or downloads at build time, third-party components under their own licenses.
The built `.app` bundles redistribute some of these; if you distribute the
apps, you are redistributing these components and must comply with their terms.

## Bundled in this repository

### winevdm / otvdm — `resources/otvdm/`
16-bit Windows runtime (used to run Win16 games such as CivNet and
Colonization). Includes Wine 16-bit DLLs.
- Project: https://github.com/otya128/winevdm
- License: GNU Lesser General Public License, version 2.1 (LGPL-2.1), with the
  underlying Wine components also under LGPL-2.1.

### libogg, libopus, opusfile — statically linked into `mcicda.dll`
The CD-audio replacement DLL (`resources/mcicda.dll`, a prebuilt binary from
https://github.com/jowtron/mcicda-stub) statically links these Xiph.Org
libraries. Their license texts are in `mcicda/licenses/`.
- libogg: https://gitlab.xiph.org/xiph/ogg (BSD 3-Clause,
  `mcicda/licenses/libogg-COPYING`)
- libopus: https://gitlab.xiph.org/xiph/opus (BSD 3-Clause,
  `mcicda/licenses/libopus-COPYING`)
- opusfile: https://gitlab.xiph.org/xiph/opusfile (BSD 3-Clause,
  `mcicda/licenses/opusfile-COPYING`)

## Downloaded / bundled at build time (not in this repository)

### Wine
Downloaded per build and bundled into every `.app`.
- Project: https://www.winehq.org/  (via https://github.com/Gcenx/macOS_Wine_builds)
- License: GNU Lesser General Public License, version 2.1 (LGPL-2.1).

### cnc-ddraw
A DirectDraw re-implementation bundled into the Alpha Centauri (SMAC) app to
make it render on modern Wine. Fetched into the SMAC game master at prep time.
- Project: https://github.com/FunkyFr3sh/cnc-ddraw
- License: GNU General Public License (GPL). Because SMAC's app redistributes
  cnc-ddraw, that app as distributed is subject to the GPL for that component.

## Not distributed by this project

Game executables, data, patches, box-art-derived icons, and CD audio are
copyrighted by their respective publishers (2K / MicroProse / Firaxis, etc.).
You must supply your own legally-obtained copies. The CivNet 1.0.2 patch is
resolved at build time from your own copy or a download — it is never committed
here (see `external_patches.go` and `resources/patches/README.md`).
