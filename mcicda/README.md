# mcicda - CD Audio Replacement DLL for Wine

A Windows DLL that intercepts MCI `cdaudio` commands and plays audio files directly via the `waveOut` API. Replaces Wine's built-in `mcicda.dll` to provide CD audio emulation from ripped audio tracks.

## Versions

| File | Size | Description |
|------|------|-------------|
| `mcicda-v2.dll` | ~370 KB | **Current.** Plays audio in-process via waveOut. Supports WAV, FLAC, MP3, OGG Vorbis, Opus. |
| `mcicda-v1.dll` | 11 KB | **Legacy.** Logging-only stub that writes commands to `C:\mcicda_commands.log`. Requires the companion [cdaudio-controller](../cdaudio-controller/) to actually play audio. |

The v2 DLL is the one bundled into `.app` builds (at `resources/mcicda.dll`).

## How it works

1. Game sends MCI commands: `open cdaudio`, `play cdaudio from X to Y`, `stop cdaudio`, etc.
2. The DLL intercepts these via the MCI driver interface
3. Maps CD track numbers to audio files at `C:\music\trackNN.{flac,wav,mp3,ogg,opus}`
4. Decodes and plays audio in-process using the Windows `waveOut` API

Audio decoding uses header-only libraries (no external dependencies):
- WAV/FLAC/MP3: [dr_libs](https://github.com/mackron/dr_libs) (dr_wav.h, dr_flac.h, dr_mp3.h)
- OGG Vorbis: [stb_vorbis](https://github.com/nothings/stb) (stb_vorbis.c)
- Opus: vendored [libopus](https://opus-codec.org/) + [libogg](https://xiph.org/ogg/) + [opusfile](https://opus-codec.org/)

## Installation

**Must be installed in BOTH locations** (Wine loads 32-bit DLLs from syswow64):

```
wineprefix/drive_c/windows/system32/mcicda.dll
wineprefix/drive_c/windows/syswow64/mcicda.dll
```

Set the DLL override so Wine uses the native version:

```bash
export WINEDLLOVERRIDES="mcicda=n"
```

## Building

Requires MinGW cross-compiler:

```bash
brew install mingw-w64
```

Full source and build instructions at [github.com/jowtron/mcicda-stub](https://github.com/jowtron/mcicda-stub).

The main source file is `mcicda_stub.c`. Header-only decoders (`dr_*.h`, `stb_vorbis.c`) and vendored Opus libraries (`deps/`) are in the full repo.
