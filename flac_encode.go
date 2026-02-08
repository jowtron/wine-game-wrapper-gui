package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"
	"github.com/mewkiz/flac/meta"
)

const flacBlockSize = 4096 // samples per frame - good balance of speed and compression

// encodeWAVToFLAC converts a WAV file to FLAC using pure Go.
func encodeWAVToFLAC(wavPath, flacPath string) error {
	// Open and parse WAV
	wavFile, err := os.Open(wavPath)
	if err != nil {
		return fmt.Errorf("open WAV: %w", err)
	}
	defer wavFile.Close()

	sampleRate, bitsPerSample, numChannels, dataSize, err := parseWAVHeader(wavFile)
	if err != nil {
		return fmt.Errorf("parse WAV: %w", err)
	}

	bytesPerSample := int(bitsPerSample) / 8
	totalSamples := uint64(dataSize) / uint64(numChannels) / uint64(bytesPerSample)

	// Create FLAC encoder
	info := &meta.StreamInfo{
		BlockSizeMin:  flacBlockSize,
		BlockSizeMax:  flacBlockSize,
		SampleRate:    uint32(sampleRate),
		NChannels:     uint8(numChannels),
		BitsPerSample: uint8(bitsPerSample),
		NSamples:      totalSamples,
	}

	outFile, err := os.Create(flacPath)
	if err != nil {
		return fmt.Errorf("create FLAC: %w", err)
	}
	defer outFile.Close()

	enc, err := flac.NewEncoder(outFile, info)
	if err != nil {
		return fmt.Errorf("create encoder: %w", err)
	}
	defer enc.Close()

	// Read and encode samples in blocks
	samplesRemaining := totalSamples
	rawBuf := make([]byte, flacBlockSize*int(numChannels)*bytesPerSample)

	for samplesRemaining > 0 {
		blockSamples := uint64(flacBlockSize)
		if blockSamples > samplesRemaining {
			blockSamples = samplesRemaining
		}

		bytesToRead := int(blockSamples) * int(numChannels) * bytesPerSample
		n, err := io.ReadFull(wavFile, rawBuf[:bytesToRead])
		if err != nil && err != io.ErrUnexpectedEOF {
			return fmt.Errorf("read samples: %w", err)
		}
		if n == 0 {
			break
		}
		actualSamples := n / (int(numChannels) * bytesPerSample)

		// Build FLAC frame
		f := &frame.Frame{
			Header: frame.Header{
				BlockSize:     uint16(actualSamples),
				SampleRate:    uint32(sampleRate),
				Channels:      frame.ChannelsLR,
				BitsPerSample: uint8(bitsPerSample),
			},
		}

		if numChannels == 1 {
			f.Header.Channels = frame.ChannelsMono
		}

		// Deinterleave samples into per-channel subframes
		for ch := 0; ch < int(numChannels); ch++ {
			samples := make([]int32, actualSamples)
			for s := 0; s < actualSamples; s++ {
				offset := (s*int(numChannels) + ch) * bytesPerSample
				switch bitsPerSample {
				case 16:
					samples[s] = int32(int16(binary.LittleEndian.Uint16(rawBuf[offset:])))
				case 24:
					val := int32(rawBuf[offset]) | int32(rawBuf[offset+1])<<8 | int32(rawBuf[offset+2])<<16
					if val&0x800000 != 0 {
						val |= ^0xFFFFFF // sign extend
					}
					samples[s] = val
				case 8:
					samples[s] = int32(rawBuf[offset]) - 128 // unsigned to signed
				}
			}

			subframe := &frame.Subframe{
				SubHeader: frame.SubHeader{
					Pred: frame.PredVerbatim,
				},
				NSamples: actualSamples,
				Samples:  samples,
			}
			f.Subframes = append(f.Subframes, subframe)
		}

		if err := enc.WriteFrame(f); err != nil {
			return fmt.Errorf("encode frame: %w", err)
		}

		samplesRemaining -= uint64(actualSamples)
	}

	return nil
}

// parseWAVHeader reads and validates a WAV file header, returning audio parameters.
// Leaves the reader positioned at the start of audio data.
func parseWAVHeader(r io.ReadSeeker) (sampleRate, bitsPerSample, numChannels uint16, dataSize uint32, err error) {
	// Read RIFF header
	var riffHeader [4]byte
	if _, err = io.ReadFull(r, riffHeader[:]); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("read RIFF: %w", err)
	}
	if string(riffHeader[:]) != "RIFF" {
		return 0, 0, 0, 0, fmt.Errorf("not a RIFF file")
	}

	// Skip file size
	var fileSize uint32
	binary.Read(r, binary.LittleEndian, &fileSize)

	// Read WAVE
	var wave [4]byte
	io.ReadFull(r, wave[:])
	if string(wave[:]) != "WAVE" {
		return 0, 0, 0, 0, fmt.Errorf("not a WAVE file")
	}

	// Read chunks until we find "fmt " and "data"
	var fmtFound, dataFound bool
	for !dataFound {
		var chunkID [4]byte
		if _, err = io.ReadFull(r, chunkID[:]); err != nil {
			return 0, 0, 0, 0, fmt.Errorf("read chunk: %w", err)
		}
		var chunkSize uint32
		if err = binary.Read(r, binary.LittleEndian, &chunkSize); err != nil {
			return 0, 0, 0, 0, fmt.Errorf("read chunk size: %w", err)
		}

		switch string(chunkID[:]) {
		case "fmt ":
			var audioFormat uint16
			binary.Read(r, binary.LittleEndian, &audioFormat)
			binary.Read(r, binary.LittleEndian, &numChannels)
			var sr uint32
			binary.Read(r, binary.LittleEndian, &sr)
			sampleRate = uint16(sr)
			var byteRate uint32
			binary.Read(r, binary.LittleEndian, &byteRate)
			var blockAlign uint16
			binary.Read(r, binary.LittleEndian, &blockAlign)
			binary.Read(r, binary.LittleEndian, &bitsPerSample)
			// Skip any extra fmt bytes
			remaining := int64(chunkSize) - 16
			if remaining > 0 {
				r.Seek(remaining, io.SeekCurrent)
			}
			fmtFound = true

		case "data":
			if !fmtFound {
				return 0, 0, 0, 0, fmt.Errorf("data chunk before fmt chunk")
			}
			dataSize = chunkSize
			dataFound = true
			// Reader is now positioned at start of audio data

		default:
			// Skip unknown chunks
			// Pad to even boundary
			skip := int64(chunkSize)
			if skip%2 != 0 {
				skip++
			}
			r.Seek(skip, io.SeekCurrent)
		}
	}

	return sampleRate, bitsPerSample, numChannels, dataSize, nil
}

// convertToFLAC converts WAV files in dir to FLAC using pure Go, removing the originals.
// Returns the number of tracks converted.
func convertToFLAC(dir string, r ProgressReporter) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".wav") {
			continue
		}
		// Skip track01 - it's the data track
		if strings.HasPrefix(strings.ToLower(name), "track01") {
			continue
		}

		wavPath := filepath.Join(dir, name)
		flacName := name[:len(name)-4] + ".flac"
		flacPath := filepath.Join(dir, flacName)

		r.Logf("  Converting %s -> %s", name, flacName)
		if err := encodeWAVToFLAC(wavPath, flacPath); err != nil {
			return count, fmt.Errorf("convert %s: %w", name, err)
		}

		// Remove the WAV
		os.Remove(wavPath)
		count++
	}

	return count, nil
}
