# mcicda - CD Audio Replacement DLL for Wine

A Windows DLL that intercepts MCI `cdaudio` commands and plays audio files directly via the `waveOut` API. Replaces Wine's built-in `mcicda.dll` to provide CD audio emulation from ripped audio tracks.

**Source, build workflow and releases live in [jowtron/mcicda-stub](https://github.com/jowtron/mcicda-stub).** This directory holds only prebuilt binaries and the license texts of the libraries statically linked into them; nothing here is built.

## Versions

| File | Size | Description |
|------|------|-------------|
| `mcicda-v2.dll` | ~360 KB | **Current.** [mcicda-stub v2.0.1](https://github.com/jowtron/mcicda-stub/releases/tag/v2.0.1). Plays audio in-process via waveOut. Supports WAV, FLAC, MP3, OGG Vorbis, Opus. |
| `mcicda-v1.dll` | 11 KB | **Legacy.** Logging-only stub that writes commands to `C:\mcicda_commands.log`. Requires the companion [cdaudio-controller](../cdaudio-controller/) to actually play audio. |

The v2 DLL is the one bundled into `.app` builds (at `resources/mcicda.dll`, embedded at Go compile time).

## Updating

1. Cut a release in mcicda-stub (its Actions workflow builds the DLL with MSVC 2022, Win32).
2. Copy the release's `mcicda.dll` over both `mcicda/mcicda-v2.dll` and `resources/mcicda.dll`, and update the version in the table above.
3. Rebuild the Go binary so the new DLL is embedded, then rebuild a game with CD audio and listen.

## How it works

1. Game sends MCI commands: `open cdaudio`, `play cdaudio from X to Y`, `stop cdaudio`, etc.
2. The DLL intercepts these via the MCI driver interface
3. Maps CD track numbers to audio files at `C:\music\trackNN.{flac,wav,mp3,ogg,opus}`
4. Decodes and plays audio in-process using the Windows `waveOut` API

It logs every command to `C:\mcicda_commands.log` in the prefix (truncated at the start of each session since v2.0.1).

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

## Licenses

mcicda-stub itself is MIT. The DLL statically links these Xiph.Org libraries, all BSD 3-Clause; their license texts are in [`licenses/`](licenses) and must accompany any redistribution of the DLL (including inside a built `.app`):

- libogg: `licenses/libogg-COPYING`
- libopus: `licenses/libopus-COPYING`
- opusfile: `licenses/opusfile-COPYING`

The WAV/FLAC/MP3 (dr_libs) and OGG Vorbis (stb_vorbis) decoders are public domain.
