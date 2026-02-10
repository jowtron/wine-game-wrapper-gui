#include <windows.h>

/*
 * keyhook.exe - Loads keyremap.dll and installs a global keyboard hook.
 * Stays resident until its window receives WM_CLOSE (sent by wineserver -k).
 *
 * Usage: keyhook.exe
 * The launcher script starts this in the background before the game.
 */

typedef BOOL (*InstallFunc)(void);
typedef void (*UninstallFunc)(void);

int WINAPI WinMain(HINSTANCE hInst, HINSTANCE hPrev, LPSTR cmdLine, int show)
{
    HMODULE hDll = LoadLibraryA("keyremap.dll");
    if (!hDll) return 1;

    InstallFunc install = (InstallFunc)GetProcAddress(hDll, "Install");
    if (!install || !install()) {
        FreeLibrary(hDll);
        return 2;
    }

    /* Pump messages to keep the DLL loaded and hook active.
     * This process exits when wineserver shuts down. */
    MSG msg;
    while (GetMessageA(&msg, NULL, 0, 0)) {
        TranslateMessage(&msg);
        DispatchMessageA(&msg);
    }

    UninstallFunc uninstall = (UninstallFunc)GetProcAddress(hDll, "Uninstall");
    if (uninstall) uninstall();
    FreeLibrary(hDll);
    return 0;
}
