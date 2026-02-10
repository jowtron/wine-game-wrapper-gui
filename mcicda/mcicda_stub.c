/*
 * MCI CD Audio Driver with Direct Multi-Format Playback
 * Intercepts CD audio commands and plays audio files using waveOut API.
 * Supports WAV, FLAC, MP3, OGG Vorbis, and Opus.
 * Uses dynamic loading of winmm.dll to avoid import issues.
 */

#include <windows.h>
#include <mmsystem.h>
#include <stdio.h>
#include <stdarg.h>
#include <string.h>

/* Single-header audio decoders - implementations */
#define DR_WAV_IMPLEMENTATION
#include "dr_wav.h"

#define DR_FLAC_IMPLEMENTATION
#include "dr_flac.h"

#define DR_MP3_IMPLEMENTATION
#include "dr_mp3.h"

#include "stb_vorbis.c"

#include <opusfile.h>

/* Internal MCI driver message IDs */
#ifndef MCI_OPEN_DRIVER
#define MCI_OPEN_DRIVER 0x0801
#endif
#ifndef MCI_CLOSE_DRIVER
#define MCI_CLOSE_DRIVER 0x0802
#endif

/* Paths */
#define MUSIC_DIR "C:\\music\\"
#define LOG_FILE "C:\\mcicda_commands.log"

/* Supported audio formats */
typedef enum {
    AUDIO_FMT_UNKNOWN = 0,
    AUDIO_FMT_WAV,
    AUDIO_FMT_FLAC,
    AUDIO_FMT_MP3,
    AUDIO_FMT_OGG,
    AUDIO_FMT_OPUS
} AudioFormat;

static const char* g_extensions[] = { ".wav", ".flac", ".mp3", ".ogg", ".opus", NULL };
static const AudioFormat g_formats[] = { AUDIO_FMT_WAV, AUDIO_FMT_FLAC, AUDIO_FMT_MP3, AUDIO_FMT_OGG, AUDIO_FMT_OPUS };

/* Device state */
static BOOL g_bOpen = FALSE;
static DWORD g_dwCurrentTrack = 2;
static DWORD g_dwNumTracks = 18;
static DWORD g_dwTimeFormat = MCI_FORMAT_TMSF;
static BOOL g_bPlaying = FALSE;
static BOOL g_bPaused = FALSE;

/* Audio playback state */
static HMODULE g_hWinMM = NULL;
static HWAVEOUT g_hWaveOut = NULL;
static WAVEHDR g_waveHdr = {0};
static BYTE* g_pAudioData = NULL;
static HANDLE g_hPlayThread = NULL;
static volatile BOOL g_bStopRequested = FALSE;

/* Function pointers for winmm.dll */
typedef MMRESULT (WINAPI *pfnWaveOutOpen)(LPHWAVEOUT, UINT, LPCWAVEFORMATEX, DWORD_PTR, DWORD_PTR, DWORD);
typedef MMRESULT (WINAPI *pfnWaveOutClose)(HWAVEOUT);
typedef MMRESULT (WINAPI *pfnWaveOutPrepareHeader)(HWAVEOUT, LPWAVEHDR, UINT);
typedef MMRESULT (WINAPI *pfnWaveOutUnprepareHeader)(HWAVEOUT, LPWAVEHDR, UINT);
typedef MMRESULT (WINAPI *pfnWaveOutWrite)(HWAVEOUT, LPWAVEHDR, UINT);
typedef MMRESULT (WINAPI *pfnWaveOutReset)(HWAVEOUT);
typedef MMRESULT (WINAPI *pfnWaveOutPause)(HWAVEOUT);
typedef MMRESULT (WINAPI *pfnWaveOutRestart)(HWAVEOUT);

static pfnWaveOutOpen pWaveOutOpen = NULL;
static pfnWaveOutClose pWaveOutClose = NULL;
static pfnWaveOutPrepareHeader pWaveOutPrepareHeader = NULL;
static pfnWaveOutUnprepareHeader pWaveOutUnprepareHeader = NULL;
static pfnWaveOutWrite pWaveOutWrite = NULL;
static pfnWaveOutReset pWaveOutReset = NULL;
static pfnWaveOutPause pWaveOutPause = NULL;
static pfnWaveOutRestart pWaveOutRestart = NULL;

/* Write a command to the log file */
static void LogCommand(const char* fmt, ...)
{
    FILE* f = fopen(LOG_FILE, "a");
    if (f) {
        va_list args;
        va_start(args, fmt);
        vfprintf(f, fmt, args);
        va_end(args);
        fprintf(f, "\n");
        fflush(f);
        fclose(f);
    }
}

/* Initialize winmm.dll function pointers */
static BOOL InitWinMM(void)
{
    if (g_hWinMM) return TRUE;

    g_hWinMM = LoadLibraryA("winmm.dll");
    if (!g_hWinMM) {
        LogCommand("ERROR: Cannot load winmm.dll");
        return FALSE;
    }

    pWaveOutOpen = (pfnWaveOutOpen)GetProcAddress(g_hWinMM, "waveOutOpen");
    pWaveOutClose = (pfnWaveOutClose)GetProcAddress(g_hWinMM, "waveOutClose");
    pWaveOutPrepareHeader = (pfnWaveOutPrepareHeader)GetProcAddress(g_hWinMM, "waveOutPrepareHeader");
    pWaveOutUnprepareHeader = (pfnWaveOutUnprepareHeader)GetProcAddress(g_hWinMM, "waveOutUnprepareHeader");
    pWaveOutWrite = (pfnWaveOutWrite)GetProcAddress(g_hWinMM, "waveOutWrite");
    pWaveOutReset = (pfnWaveOutReset)GetProcAddress(g_hWinMM, "waveOutReset");
    pWaveOutPause = (pfnWaveOutPause)GetProcAddress(g_hWinMM, "waveOutPause");
    pWaveOutRestart = (pfnWaveOutRestart)GetProcAddress(g_hWinMM, "waveOutRestart");

    if (!pWaveOutOpen || !pWaveOutClose || !pWaveOutPrepareHeader ||
        !pWaveOutUnprepareHeader || !pWaveOutWrite || !pWaveOutReset) {
        LogCommand("ERROR: Cannot get winmm function pointers");
        FreeLibrary(g_hWinMM);
        g_hWinMM = NULL;
        return FALSE;
    }

    LogCommand("winmm.dll loaded successfully");
    return TRUE;
}

/* Build path to track file, trying multiple extensions.
 * Returns the detected format, or AUDIO_FMT_UNKNOWN if no file found. */
static AudioFormat GetTrackPath(DWORD track, char* path, size_t pathSize)
{
    int i;
    for (i = 0; g_extensions[i] != NULL; i++) {
        _snprintf(path, pathSize, "%strack%02d%s", MUSIC_DIR, track, g_extensions[i]);
        if (GetFileAttributesA(path) != INVALID_FILE_ATTRIBUTES) {
            return g_formats[i];
        }
    }
    /* No file found */
    path[0] = '\0';
    return AUDIO_FMT_UNKNOWN;
}

/* Check if track exists in any supported format */
static BOOL TrackExists(DWORD track)
{
    char path[MAX_PATH];
    return GetTrackPath(track, path, MAX_PATH) != AUDIO_FMT_UNKNOWN;
}

/* Decode audio file to 16-bit PCM.
 * Returns allocated buffer (caller must free), or NULL on failure.
 * Sets channels, sampleRate, and totalSamples (total int16 samples = frames * channels). */
static short* DecodeAudioFile(const char* path, AudioFormat fmt,
                              unsigned int* channels, unsigned int* sampleRate,
                              size_t* totalSamples)
{
    short* pcm = NULL;

    switch (fmt) {
    case AUDIO_FMT_WAV: {
        drwav_uint64 frameCount = 0;
        pcm = drwav_open_file_and_read_pcm_frames_s16(path, channels, sampleRate, &frameCount, NULL);
        if (pcm) {
            *totalSamples = (size_t)(frameCount * (*channels));
            LogCommand("Decoded WAV: %uch %uHz, %llu frames", *channels, *sampleRate, (unsigned long long)frameCount);
        }
        break;
    }
    case AUDIO_FMT_FLAC: {
        drflac_uint64 frameCount = 0;
        pcm = drflac_open_file_and_read_pcm_frames_s16(path, channels, sampleRate, &frameCount, NULL);
        if (pcm) {
            *totalSamples = (size_t)(frameCount * (*channels));
            LogCommand("Decoded FLAC: %uch %uHz, %llu frames", *channels, *sampleRate, (unsigned long long)frameCount);
        }
        break;
    }
    case AUDIO_FMT_MP3: {
        drmp3_config config = {0};
        drmp3_uint64 frameCount = 0;
        pcm = drmp3_open_file_and_read_pcm_frames_s16(path, &config, &frameCount, NULL);
        if (pcm) {
            *channels = config.channels;
            *sampleRate = config.sampleRate;
            *totalSamples = (size_t)(frameCount * config.channels);
            LogCommand("Decoded MP3: %uch %uHz, %llu frames", *channels, *sampleRate, (unsigned long long)frameCount);
        }
        break;
    }
    case AUDIO_FMT_OGG: {
        int ch = 0, sr = 0;
        short* output = NULL;
        int sampleFrames = stb_vorbis_decode_filename(path, &ch, &sr, &output);
        if (sampleFrames > 0 && output) {
            pcm = output;
            *channels = (unsigned int)ch;
            *sampleRate = (unsigned int)sr;
            *totalSamples = (size_t)sampleFrames * (size_t)ch;
            LogCommand("Decoded OGG: %uch %uHz, %d frames", *channels, *sampleRate, sampleFrames);
        }
        break;
    }
    case AUDIO_FMT_OPUS: {
        int error = 0;
        OggOpusFile* of = op_open_file(path, &error);
        if (of) {
            ogg_int64_t totalPcmFrames = op_pcm_total(of, -1);
            int ch = op_channel_count(of, -1);
            /* Opus always decodes at 48000 Hz */
            unsigned int sr = 48000;
            if (totalPcmFrames > 0 && ch > 0) {
                size_t totalSamp = (size_t)totalPcmFrames * (size_t)ch;
                short* buf = (short*)malloc(totalSamp * sizeof(short));
                if (buf) {
                    size_t filled = 0;
                    int link_index;
                    while (filled < totalSamp) {
                        int ret = op_read(of, buf + filled, (int)(totalSamp - filled), &link_index);
                        if (ret <= 0) break;
                        filled += (size_t)ret * (size_t)ch;
                    }
                    if (filled > 0) {
                        pcm = buf;
                        *channels = (unsigned int)ch;
                        *sampleRate = sr;
                        *totalSamples = filled;
                        LogCommand("Decoded Opus: %uch %uHz, %llu frames", *channels, *sampleRate,
                                   (unsigned long long)(filled / ch));
                    } else {
                        free(buf);
                    }
                }
            }
            op_free(of);
        } else {
            LogCommand("ERROR: op_open_file failed (%d)", error);
        }
        break;
    }
    default:
        break;
    }

    return pcm;
}

/* Stop current playback */
static void StopPlayback(void)
{
    g_bStopRequested = TRUE;

    if (g_hPlayThread) {
        WaitForSingleObject(g_hPlayThread, 2000);
        CloseHandle(g_hPlayThread);
        g_hPlayThread = NULL;
    }

    if (g_hWaveOut && pWaveOutReset && pWaveOutUnprepareHeader && pWaveOutClose) {
        pWaveOutReset(g_hWaveOut);
        if (g_waveHdr.dwFlags & WHDR_PREPARED) {
            pWaveOutUnprepareHeader(g_hWaveOut, &g_waveHdr, sizeof(WAVEHDR));
        }
        pWaveOutClose(g_hWaveOut);
        g_hWaveOut = NULL;
    }

    if (g_pAudioData) {
        free(g_pAudioData);
        g_pAudioData = NULL;
    }

    ZeroMemory(&g_waveHdr, sizeof(g_waveHdr));
    g_bPlaying = FALSE;
    g_bPaused = FALSE;
    g_bStopRequested = FALSE;
}

/* Playback thread argument */
typedef struct {
    char path[MAX_PATH];
    AudioFormat format;
} PlaybackArgs;

/* Playback thread */
static DWORD WINAPI PlaybackThread(LPVOID param)
{
    PlaybackArgs* args = (PlaybackArgs*)param;
    unsigned int channels = 0, sampleRate = 0;
    size_t totalSamples = 0;
    short* pcmData;
    WAVEFORMATEX wfx;
    DWORD dataSize;
    MMRESULT result;

    LogCommand("PlaybackThread: %s", args->path);

    if (!InitWinMM()) {
        free(args);
        return 1;
    }

    /* Decode audio to PCM */
    pcmData = DecodeAudioFile(args->path, args->format, &channels, &sampleRate, &totalSamples);
    if (!pcmData) {
        LogCommand("ERROR: Failed to decode %s", args->path);
        free(args);
        return 1;
    }

    dataSize = (DWORD)(totalSamples * sizeof(short));
    g_pAudioData = (BYTE*)pcmData;

    /* Set up waveOut format (16-bit PCM) */
    wfx.wFormatTag = WAVE_FORMAT_PCM;
    wfx.nChannels = (WORD)channels;
    wfx.nSamplesPerSec = sampleRate;
    wfx.wBitsPerSample = 16;
    wfx.nBlockAlign = (WORD)(channels * 2);
    wfx.nAvgBytesPerSec = sampleRate * channels * 2;
    wfx.cbSize = 0;

    LogCommand("PCM: %dch %dHz 16bit %d bytes", channels, sampleRate, dataSize);

    /* Open waveOut */
    result = pWaveOutOpen(&g_hWaveOut, WAVE_MAPPER, &wfx, 0, 0, CALLBACK_NULL);
    if (result != MMSYSERR_NOERROR) {
        LogCommand("ERROR: waveOutOpen failed %d", result);
        free(args);
        free(g_pAudioData);
        g_pAudioData = NULL;
        return 1;
    }

    /* Prepare header */
    g_waveHdr.lpData = (LPSTR)g_pAudioData;
    g_waveHdr.dwBufferLength = dataSize;
    g_waveHdr.dwFlags = 0;

    result = pWaveOutPrepareHeader(g_hWaveOut, &g_waveHdr, sizeof(WAVEHDR));
    if (result != MMSYSERR_NOERROR) {
        LogCommand("ERROR: waveOutPrepareHeader failed");
        pWaveOutClose(g_hWaveOut);
        g_hWaveOut = NULL;
        free(args);
        free(g_pAudioData);
        g_pAudioData = NULL;
        return 1;
    }

    /* Start playback */
    result = pWaveOutWrite(g_hWaveOut, &g_waveHdr, sizeof(WAVEHDR));
    if (result != MMSYSERR_NOERROR) {
        LogCommand("ERROR: waveOutWrite failed");
        pWaveOutUnprepareHeader(g_hWaveOut, &g_waveHdr, sizeof(WAVEHDR));
        pWaveOutClose(g_hWaveOut);
        g_hWaveOut = NULL;
        free(args);
        free(g_pAudioData);
        g_pAudioData = NULL;
        return 1;
    }

    LogCommand("PLAYING");
    g_bPlaying = TRUE;

    /* Wait for completion */
    while (!g_bStopRequested) {
        if (g_waveHdr.dwFlags & WHDR_DONE) {
            LogCommand("PLAYBACK_DONE");
            break;
        }
        Sleep(100);
    }

    free(args);
    return 0;
}

/* Play a track */
static BOOL PlayTrack(DWORD track)
{
    char path[MAX_PATH];
    AudioFormat fmt;
    PlaybackArgs* args;

    StopPlayback();

    fmt = GetTrackPath(track, path, MAX_PATH);
    LogCommand("PLAY %d (%s)", track, path);

    if (fmt == AUDIO_FMT_UNKNOWN) {
        LogCommand("ERROR: No audio file found for track %d", track);
        return FALSE;
    }

    args = (PlaybackArgs*)malloc(sizeof(PlaybackArgs));
    if (!args) return FALSE;

    strncpy(args->path, path, MAX_PATH - 1);
    args->path[MAX_PATH - 1] = '\0';
    args->format = fmt;

    g_dwCurrentTrack = track;
    g_bStopRequested = FALSE;

    g_hPlayThread = CreateThread(NULL, 0, PlaybackThread, args, 0, NULL);
    if (!g_hPlayThread) {
        LogCommand("ERROR: CreateThread failed");
        free(args);
        return FALSE;
    }

    return TRUE;
}

/* Pause playback */
static void PausePlayback(void)
{
    if (g_hWaveOut && pWaveOutPause && g_bPlaying && !g_bPaused) {
        pWaveOutPause(g_hWaveOut);
        g_bPaused = TRUE;
        LogCommand("PAUSE");
    }
}

/* Resume playback */
static void ResumePlayback(void)
{
    if (g_hWaveOut && pWaveOutRestart && g_bPaused) {
        pWaveOutRestart(g_hWaveOut);
        g_bPaused = FALSE;
        LogCommand("RESUME");
    }
}

/* Count available tracks */
static DWORD CountTracks(void)
{
    DWORD count = 0;
    DWORD i;
    for (i = 2; i <= 99; i++) {
        if (TrackExists(i)) count++;
        else if (count > 0) break;
    }
    return count > 0 ? count + 1 : 18;
}

/* MCI driver procedure */
LRESULT CALLBACK DriverProc(DWORD_PTR dwDriverId, HDRVR hDriver, UINT msg,
                            LPARAM lParam1, LPARAM lParam2)
{
    switch (msg) {
    case DRV_LOAD:
    case DRV_ENABLE:
        return 1;
    case DRV_OPEN:
    case DRV_CLOSE:
    case DRV_DISABLE:
    case DRV_FREE:
        return 1;
    case DRV_QUERYCONFIGURE:
        return 0;
    case DRV_INSTALL:
    case DRV_REMOVE:
        return DRV_OK;
    }

    if (msg == MCI_OPEN_DRIVER) {
        g_bOpen = TRUE;
        g_dwNumTracks = CountTracks();
        LogCommand("OPEN (%d tracks)", g_dwNumTracks);
        return 0;
    }

    if (msg == MCI_CLOSE_DRIVER) {
        LogCommand("CLOSE");
        StopPlayback();
        g_bOpen = FALSE;
        return 0;
    }

    if (!g_bOpen)
        return MCIERR_DEVICE_NOT_READY;

    switch (msg) {
    case MCI_OPEN:
        LogCommand("MCI_OPEN");
        return 0;

    case MCI_CLOSE:
        LogCommand("MCI_CLOSE");
        StopPlayback();
        return 0;

    case MCI_PLAY:
        {
            DWORD dwFrom = g_dwCurrentTrack;

            if (lParam1 & MCI_FROM) {
                MCI_PLAY_PARMS* parms = (MCI_PLAY_PARMS*)lParam2;
                if (g_dwTimeFormat == MCI_FORMAT_TMSF)
                    dwFrom = MCI_TMSF_TRACK(parms->dwFrom);
                else
                    dwFrom = parms->dwFrom;
            }

            PlayTrack(dwFrom);
        }
        return 0;

    case MCI_STOP:
        LogCommand("STOP");
        StopPlayback();
        return 0;

    case MCI_PAUSE:
        PausePlayback();
        return 0;

    case MCI_RESUME:
        ResumePlayback();
        return 0;

    case MCI_SEEK:
        if (lParam1 & MCI_TO) {
            MCI_SEEK_PARMS* parms = (MCI_SEEK_PARMS*)lParam2;
            DWORD dwTrack;
            if (g_dwTimeFormat == MCI_FORMAT_TMSF)
                dwTrack = MCI_TMSF_TRACK(parms->dwTo);
            else
                dwTrack = parms->dwTo;
            g_dwCurrentTrack = dwTrack;
            LogCommand("SEEK %d", dwTrack);
        }
        return 0;

    case MCI_STATUS:
        {
            MCI_STATUS_PARMS* parms = (MCI_STATUS_PARMS*)lParam2;
            if (!parms) return MCIERR_NULL_PARAMETER_BLOCK;

            if (lParam1 & MCI_STATUS_ITEM) {
                switch (parms->dwItem) {
                case MCI_STATUS_NUMBER_OF_TRACKS:
                    parms->dwReturn = g_dwNumTracks;
                    break;
                case MCI_STATUS_CURRENT_TRACK:
                    parms->dwReturn = g_dwCurrentTrack;
                    break;
                case MCI_STATUS_LENGTH:
                    parms->dwReturn = 180000;
                    break;
                case MCI_STATUS_MODE:
                    if (g_bPlaying)
                        parms->dwReturn = g_bPaused ? MCI_MODE_PAUSE : MCI_MODE_PLAY;
                    else
                        parms->dwReturn = MCI_MODE_STOP;
                    break;
                case MCI_STATUS_MEDIA_PRESENT:
                    parms->dwReturn = TRUE;
                    break;
                case MCI_STATUS_READY:
                    parms->dwReturn = TRUE;
                    break;
                case MCI_STATUS_POSITION:
                    parms->dwReturn = MCI_MAKE_TMSF(g_dwCurrentTrack, 0, 0, 0);
                    break;
                case MCI_STATUS_TIME_FORMAT:
                    parms->dwReturn = g_dwTimeFormat;
                    break;
                case MCI_CDA_STATUS_TYPE_TRACK:
                    parms->dwReturn = MCI_CDA_TRACK_AUDIO;
                    break;
                default:
                    parms->dwReturn = 0;
                }
            }
        }
        return 0;

    case MCI_SET:
        {
            MCI_SET_PARMS* parms = (MCI_SET_PARMS*)lParam2;
            if (parms && (lParam1 & MCI_SET_TIME_FORMAT)) {
                g_dwTimeFormat = parms->dwTimeFormat;
            }
        }
        return 0;

    case MCI_GETDEVCAPS:
        {
            MCI_GETDEVCAPS_PARMS* parms = (MCI_GETDEVCAPS_PARMS*)lParam2;
            if (!parms) return MCIERR_NULL_PARAMETER_BLOCK;

            if (lParam1 & MCI_GETDEVCAPS_ITEM) {
                switch (parms->dwItem) {
                case MCI_GETDEVCAPS_CAN_PLAY:
                case MCI_GETDEVCAPS_HAS_AUDIO:
                    parms->dwReturn = TRUE;
                    break;
                case MCI_GETDEVCAPS_CAN_RECORD:
                case MCI_GETDEVCAPS_HAS_VIDEO:
                case MCI_GETDEVCAPS_CAN_EJECT:
                case MCI_GETDEVCAPS_CAN_SAVE:
                case MCI_GETDEVCAPS_USES_FILES:
                case MCI_GETDEVCAPS_COMPOUND_DEVICE:
                    parms->dwReturn = FALSE;
                    break;
                case MCI_GETDEVCAPS_DEVICE_TYPE:
                    parms->dwReturn = MCI_DEVTYPE_CD_AUDIO;
                    break;
                default:
                    parms->dwReturn = 0;
                }
            }
        }
        return 0;

    case MCI_INFO:
        if (lParam2) {
            MCI_INFO_PARMS* parms = (MCI_INFO_PARMS*)lParam2;
            if (parms->lpstrReturn && parms->dwRetSize > 0)
                parms->lpstrReturn[0] = '\0';
        }
        return 0;

    default:
        return MCIERR_UNRECOGNIZED_COMMAND;
    }

    return 0;
}

/* DLL entry point */
BOOL WINAPI DllMain(HINSTANCE hinstDLL, DWORD fdwReason, LPVOID lpvReserved)
{
    switch (fdwReason) {
    case DLL_PROCESS_ATTACH:
        DisableThreadLibraryCalls(hinstDLL);
        break;
    case DLL_PROCESS_DETACH:
        StopPlayback();
        if (g_hWinMM) {
            FreeLibrary(g_hWinMM);
            g_hWinMM = NULL;
        }
        break;
    }
    return TRUE;
}
