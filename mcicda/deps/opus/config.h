/* Minimal config.h for building libopus with MSVC (Win32, no SIMD) */
#ifndef OPUS_CONFIG_H
#define OPUS_CONFIG_H

#define OPUS_BUILD 1
#define PACKAGE_VERSION "1.5.2"

#define HAVE_STDINT_H 1
#define HAVE_STDLIB_H 1
#define HAVE_STRING_H 1
#define HAVE_STDIO_H 1

/* Use alloca on MSVC */
#define USE_ALLOCA 1

/* MSVC uses _lrintf */
#define HAVE_LRINTF 1

/* Use restrict keyword */
#define restrict __restrict

#endif
