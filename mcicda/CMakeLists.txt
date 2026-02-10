cmake_minimum_required(VERSION 3.10)
project(mcicda_stub C)

# --- libogg ---
file(GLOB OGG_SOURCES deps/ogg/src/*.c)

# --- libopus ---
file(GLOB OPUS_SRC_SOURCES deps/opus/src/*.c)
file(GLOB OPUS_CELT_SOURCES deps/opus/celt/*.c)
file(GLOB OPUS_SILK_SOURCES deps/opus/silk/*.c)
file(GLOB OPUS_SILK_FLOAT_SOURCES deps/opus/silk/float/*.c)

# --- opusfile ---
set(OPUSFILE_SOURCES
    deps/opusfile/src/info.c
    deps/opusfile/src/internal.c
    deps/opusfile/src/opusfile.c
    deps/opusfile/src/stream.c
)

# Build as 32-bit DLL
add_library(mcicda SHARED
    mcicda_stub.c
    mcicda.def
    ${OGG_SOURCES}
    ${OPUS_SRC_SOURCES}
    ${OPUS_CELT_SOURCES}
    ${OPUS_SILK_SOURCES}
    ${OPUS_SILK_FLOAT_SOURCES}
    ${OPUSFILE_SOURCES}
)

# Include paths for Opus dependencies
target_include_directories(mcicda PRIVATE
    ${CMAKE_SOURCE_DIR}/deps/ogg/include
    ${CMAKE_SOURCE_DIR}/deps/opus/include
    ${CMAKE_SOURCE_DIR}/deps/opus
    ${CMAKE_SOURCE_DIR}/deps/opus/celt
    ${CMAKE_SOURCE_DIR}/deps/opus/silk
    ${CMAKE_SOURCE_DIR}/deps/opus/silk/float
    ${CMAKE_SOURCE_DIR}/deps/opus/src
    ${CMAKE_SOURCE_DIR}/deps/opusfile/include
    ${CMAKE_SOURCE_DIR}/deps/opusfile/src
)

# Opus build defines
target_compile_definitions(mcicda PRIVATE
    HAVE_CONFIG_H
    OPUS_BUILD
    USE_ALLOCA
)

# Suppress warnings in vendored code
if(MSVC)
    target_compile_options(mcicda PRIVATE /W1)
endif()

# Link with Windows multimedia library
target_link_libraries(mcicda winmm)

# Set output name
set_target_properties(mcicda PROPERTIES
    OUTPUT_NAME "mcicda"
    PREFIX ""
    SUFFIX ".dll"
)
