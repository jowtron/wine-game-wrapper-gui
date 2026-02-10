#include <windows.h>
#include <stdio.h>
#include <stdlib.h>

/*
 * keyremap.dll - Keyboard remapping via global WH_GETMESSAGE hook
 *
 * Exports Install/Uninstall functions called by keyhook.exe.
 * When installed as a global hook, Windows injects this DLL into
 * every GUI process (including otvdm), remapping keys for Win16 games.
 *
 * Remapping table is read from C:\keyremap.ini.
 */

#pragma data_seg(".shared")
static HHOOK g_hook = NULL;
#pragma data_seg()

static HINSTANCE g_hInst = NULL;

/* Remap tables (per-process, loaded on DLL attach) */
static BYTE vk_remap[256] = {0};
static BYTE sc_remap[256] = {0};

static void load_remap_table(void)
{
    char buf[1024];
    GetPrivateProfileStringA("remap", NULL, "", buf, sizeof(buf), "C:\\keyremap.ini");

    char *key = buf;
    while (*key) {
        char val[64];
        GetPrivateProfileStringA("remap", key, "", val, sizeof(val), "C:\\keyremap.ini");
        if (val[0]) {
            unsigned int dst_vk = 0, src_sc = 0, dst_sc = 0;
            if (sscanf(val, "%x,%x,%x", &dst_vk, &src_sc, &dst_sc) >= 1) {
                unsigned int src_vk = (unsigned int)strtoul(key, NULL, 16);
                if (src_vk < 256 && dst_vk < 256)
                    vk_remap[src_vk] = (BYTE)dst_vk;
                if (src_sc < 256 && dst_sc < 256 && src_sc > 0 && dst_sc > 0)
                    sc_remap[src_sc] = (BYTE)dst_sc;
            }
        }
        key += lstrlenA(key) + 1;
    }
}

static LRESULT CALLBACK GetMsgProc(int nCode, WPARAM wParam, LPARAM lParam)
{
    if (nCode >= 0) {
        MSG *msg = (MSG *)lParam;
        if (msg->message == WM_KEYDOWN || msg->message == WM_KEYUP ||
            msg->message == WM_SYSKEYDOWN || msg->message == WM_SYSKEYUP) {
            BYTE vk = (BYTE)(msg->wParam & 0xFF);
            BYTE sc = (BYTE)((msg->lParam >> 16) & 0xFF);

            if (vk_remap[vk])
                msg->wParam = (msg->wParam & ~0xFF) | vk_remap[vk];
            if (sc_remap[sc])
                msg->lParam = (msg->lParam & ~(0xFF << 16)) | ((LPARAM)sc_remap[sc] << 16);
        }
    }
    return CallNextHookEx(g_hook, nCode, wParam, lParam);
}

__declspec(dllexport) BOOL Install(void)
{
    if (g_hook) return TRUE;
    g_hook = SetWindowsHookExA(WH_GETMESSAGE, GetMsgProc, g_hInst, 0);
    return g_hook != NULL;
}

__declspec(dllexport) void Uninstall(void)
{
    if (g_hook) {
        UnhookWindowsHookEx(g_hook);
        g_hook = NULL;
    }
}

BOOL WINAPI DllMain(HINSTANCE hInst, DWORD reason, LPVOID reserved)
{
    if (reason == DLL_PROCESS_ATTACH) {
        g_hInst = hInst;
        DisableThreadLibraryCalls(hInst);
        load_remap_table();
    }
    return TRUE;
}
