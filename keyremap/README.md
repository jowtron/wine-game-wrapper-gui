# keyremap

Keyboard remapping for Wine on macOS via a global Windows message hook. Designed for retro games (especially Win16 games running under [otvdm](https://github.com/otya128/winevdm)) where you need to remap regular keys to numpad keys for unit movement.

## Why?

Wine's macOS driver (`macdrv`) ignores the Windows `Scancode Map` registry key that normally handles low-level key remapping. This tool works around that limitation by using a `WH_GETMESSAGE` hook that Windows injects into every GUI process — including otvdm, which runs Win16 games.

## How it works

```
keyhook.exe (background)
  → loads keyremap.dll
  → calls Install() → SetWindowsHookEx(WH_GETMESSAGE, ..., 0)
  → global hook installed for ALL processes

Game launches (e.g. otvdm.exe for Win16 games)
  → Windows injects keyremap.dll into the process
  → DLL reads C:\keyremap.ini for remap config
  → hook callback intercepts WM_KEYDOWN/WM_KEYUP messages
  → remaps VK codes and scancodes before the app sees them
```

Two files are needed because Windows global hooks require a DLL (containing the hook callback) and a host process (to keep the hook alive). The DLL gets automatically injected into target processes by the OS hook mechanism.

## Building

Requires MinGW cross-compiler (32-bit Windows target):

```bash
brew install mingw-w64
```

Build all three files:

```bash
# The hook DLL (remaps keys, injected into all GUI processes)
i686-w64-mingw32-gcc -shared -o keyremap.dll keyremap.c -luser32 -lkernel32 -O2 -s

# The hook installer (loads DLL, installs global hook, stays resident)
i686-w64-mingw32-gcc -o keyhook.exe keyhook.c -luser32 -lkernel32 -mwindows -O2 -s

# Test utility (shows VK codes and scancodes in window title)
i686-w64-mingw32-gcc -o keytest.exe keytest.c -lgdi32 -luser32 -mwindows -O2 -s
```

## Usage with Wine

### 1. Install files into the Wine prefix

```bash
PREFIX="/path/to/wineprefix"

# DLL goes in system32 and syswow64
cp keyremap.dll "$PREFIX/drive_c/windows/system32/"
cp keyremap.dll "$PREFIX/drive_c/windows/syswow64/"

# Hook installer and config go in drive_c root
cp keyhook.exe "$PREFIX/drive_c/"
```

### 2. Create remap config

Create `$PREFIX/drive_c/keyremap.ini`:

```ini
[remap]
; Format: src_vk_hex=dst_vk_hex,src_scancode_hex,dst_scancode_hex
;
; Example: Remap 789/uo/jkl to numpad for CivNet unit movement
; (I key is NOT remapped - needed for irrigate command)
37=67,08,47
38=68,09,48
39=69,0A,49
55=64,16,4B
4F=66,18,4D
4A=61,24,4F
4B=62,25,50
4C=63,26,51
```

### 3. Launch

```bash
export WINEPREFIX="$PREFIX"
export WINEDLLOVERRIDES="keyremap=n"
export WINEDEBUG=-all

# Start hook installer in background
wine "C:\keyhook.exe" &
sleep 1

# Launch game
wine GAME.EXE
```

The `WINEDLLOVERRIDES="keyremap=n"` tells Wine to use the native (our) DLL instead of looking for a builtin.

### 4. Testing

Use `keytest.exe` to verify remapping without launching a game:

```bash
# Terminal 1: start hook
WINEPREFIX="$PREFIX" WINEDLLOVERRIDES="keyremap=n" wine "C:\keyhook.exe" &

# Terminal 2: start key tester
WINEPREFIX="$PREFIX" wine keytest.exe
```

Press keys and check the window title — it shows the VK code and scancode for each keypress. If remapping is active, you'll see the remapped codes (e.g. pressing `7` shows `VK=0x67` which is `VK_NUMPAD7`).

## Common VK codes and scancodes

| Key | VK Code | Scancode | Numpad Key | VK Code | Scancode |
|-----|---------|----------|------------|---------|----------|
| 7   | 0x37    | 0x08     | Numpad 7   | 0x67    | 0x47     |
| 8   | 0x38    | 0x09     | Numpad 8   | 0x68    | 0x48     |
| 9   | 0x39    | 0x0A     | Numpad 9   | 0x69    | 0x49     |
| U   | 0x55    | 0x16     | Numpad 4   | 0x64    | 0x4B     |
| I   | 0x49    | 0x17     | Numpad 5   | 0x65    | 0x4C     |
| O   | 0x4F    | 0x18     | Numpad 6   | 0x66    | 0x4D     |
| J   | 0x4A    | 0x24     | Numpad 1   | 0x61    | 0x4F     |
| K   | 0x4B    | 0x25     | Numpad 2   | 0x62    | 0x50     |
| L   | 0x4C    | 0x26     | Numpad 3   | 0x63    | 0x51     |

## Integration with wine-game-wrapper-gui

This tool is bundled into [wine-game-wrapper-gui](https://github.com/jowtron/wine-game-wrapper-gui) and automatically installed when a game profile has key remappings defined. The launcher script starts `keyhook.exe` before the game and the config is generated from the profile's `ScancodeMap` entries.

## Files

| File | Size | Description |
|------|------|-------------|
| `keyremap.c` | Source | Hook DLL — remap logic + INI config reader |
| `keyhook.c` | Source | Hook installer — loads DLL, stays resident |
| `keytest.c` | Source | Test utility — displays key codes in window title |
| `keyremap.dll` | ~11 KB | Compiled hook DLL |
| `keyhook.exe` | ~13 KB | Compiled hook installer |
| `keytest.exe` | ~15 KB | Compiled test utility |
