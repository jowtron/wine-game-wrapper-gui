#include <windows.h>
#include <stdio.h>

/*
 * keytest.exe - Displays key presses with VK codes and scancodes.
 * Optionally loads keyremap.dll if found next to the exe.
 */

static LRESULT CALLBACK WndProc(HWND hwnd, UINT msg, WPARAM wParam, LPARAM lParam)
{
    switch (msg) {
    case WM_KEYDOWN:
    case WM_SYSKEYDOWN: {
        BYTE vk = (BYTE)(wParam & 0xFF);
        BYTE sc = (BYTE)((lParam >> 16) & 0xFF);
        char buf[256];
        sprintf(buf, "DOWN  VK=0x%02X  SC=0x%02X  '%c'", vk, sc,
                (vk >= 0x20 && vk < 0x7F) ? (char)vk : '?');
        SetWindowTextA(hwnd, buf);
        break;
    }
    case WM_KEYUP:
    case WM_SYSKEYUP: {
        BYTE vk = (BYTE)(wParam & 0xFF);
        BYTE sc = (BYTE)((lParam >> 16) & 0xFF);
        char buf[256];
        sprintf(buf, "UP    VK=0x%02X  SC=0x%02X  '%c'", vk, sc,
                (vk >= 0x20 && vk < 0x7F) ? (char)vk : '?');
        SetWindowTextA(hwnd, buf);
        break;
    }
    case WM_DESTROY:
        PostQuitMessage(0);
        return 0;
    }
    return DefWindowProcA(hwnd, msg, wParam, lParam);
}

int WINAPI WinMain(HINSTANCE hInst, HINSTANCE hPrev, LPSTR cmdLine, int show)
{
    WNDCLASSA wc = {0};
    wc.lpfnWndProc = WndProc;
    wc.hInstance = hInst;
    wc.lpszClassName = "KeyTest";
    wc.hbrBackground = (HBRUSH)(COLOR_WINDOW + 1);
    RegisterClassA(&wc);

    HWND hwnd = CreateWindowA("KeyTest", "KeyTest - Press keys!",
        WS_OVERLAPPEDWINDOW | WS_VISIBLE,
        100, 100, 500, 200, NULL, NULL, hInst, NULL);

    MSG msg;
    while (GetMessageA(&msg, NULL, 0, 0)) {
        TranslateMessage(&msg);
        DispatchMessageA(&msg);
    }

    return 0;
}
