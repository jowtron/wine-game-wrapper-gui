package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// cueTrack represents a single track parsed from a CUE sheet.
type cueTrack struct {
	Number   int
	Type     string // "MODE1/2352", "MODE2/2352", "AUDIO", etc.
	StartLBA int    // Logical block address (from INDEX 01)
}

// parseCUE parses a CUE sheet and returns the list of tracks.
func parseCUE(cuePath string) ([]cueTrack, error) {
	f, err := os.Open(cuePath)
	if err != nil {
		return nil, fmt.Errorf("open CUE file: %w", err)
	}
	defer f.Close()

	var tracks []cueTrack
	var current *cueTrack

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		upper := strings.ToUpper(line)

		if strings.HasPrefix(upper, "TRACK ") {
			parts := strings.Fields(line)
			if len(parts) < 3 {
				continue
			}
			num, err := strconv.Atoi(parts[1])
			if err != nil {
				continue
			}
			t := cueTrack{Number: num, Type: strings.ToUpper(parts[2])}
			tracks = append(tracks, t)
			current = &tracks[len(tracks)-1]
		} else if strings.HasPrefix(upper, "INDEX 01 ") && current != nil {
			parts := strings.Fields(line)
			if len(parts) < 3 {
				continue
			}
			lba, err := parseMSF(parts[2])
			if err != nil {
				continue
			}
			current.StartLBA = lba
		}
	}
	return tracks, scanner.Err()
}

// parseMSF converts MM:SS:FF (minutes:seconds:frames) to LBA (logical block address).
// Each second = 75 frames. Each frame = 2352 bytes (one sector).
func parseMSF(msf string) (int, error) {
	parts := strings.Split(msf, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid MSF: %s", msf)
	}
	m, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	s, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	f, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, err
	}
	return (m*60+s)*75 + f, nil
}

// isDataTrack returns true if the track type indicates a data (ISO) track.
func isDataTrack(trackType string) bool {
	return strings.HasPrefix(trackType, "MODE1") || strings.HasPrefix(trackType, "MODE2")
}

const sectorSize = 2352

// splitCUEBIN splits a BIN file into individual track files according to the CUE sheet.
// Data tracks are written as raw ISO (with 16-byte header stripped for MODE1/2352).
// Audio tracks are written as WAV files.
func splitCUEBIN(binPath, cuePath, outputDir string, r ProgressReporter) error {
	tracks, err := parseCUE(cuePath)
	if err != nil {
		return err
	}
	if len(tracks) == 0 {
		return fmt.Errorf("no tracks found in CUE file")
	}

	binFile, err := os.Open(binPath)
	if err != nil {
		return fmt.Errorf("open BIN: %w", err)
	}
	defer binFile.Close()

	binInfo, err := binFile.Stat()
	if err != nil {
		return err
	}
	totalSectors := int(binInfo.Size()) / sectorSize

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	count := 0
	for i, track := range tracks {
		// Determine end LBA (start of next track, or end of file)
		endLBA := totalSectors
		if i+1 < len(tracks) {
			endLBA = tracks[i+1].StartLBA
		}

		numSectors := endLBA - track.StartLBA
		if numSectors <= 0 {
			continue
		}

		// Seek to track start
		offset := int64(track.StartLBA) * sectorSize
		if _, err := binFile.Seek(offset, io.SeekStart); err != nil {
			return fmt.Errorf("seek to track %d: %w", track.Number, err)
		}

		if isDataTrack(track.Type) {
			outName := fmt.Sprintf("track%02d.iso", track.Number)
			outPath := filepath.Join(outputDir, outName)
			if err := writeDataTrack(binFile, outPath, track.Type, numSectors); err != nil {
				return fmt.Errorf("write track %d: %w", track.Number, err)
			}
			r.Logf("  Track %02d: %s (%d sectors)", track.Number, outName, numSectors)
		} else {
			outName := fmt.Sprintf("track%02d.wav", track.Number)
			outPath := filepath.Join(outputDir, outName)
			if err := writeAudioTrack(binFile, outPath, numSectors); err != nil {
				return fmt.Errorf("write track %d: %w", track.Number, err)
			}
			r.Logf("  Track %02d: %s (%d sectors)", track.Number, outName, numSectors)
		}
		count++
	}

	r.Logf("  Extracted %d tracks", count)
	return nil
}

// writeDataTrack extracts a data track from the BIN file as an ISO.
// For MODE1/2352 sectors: each sector has a 16-byte header, 2048 bytes of user data,
// and 288 bytes of ECC/EDC. We extract just the 2048-byte user data.
func writeDataTrack(binFile *os.File, outPath, trackType string, numSectors int) error {
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	buf := make([]byte, sectorSize)
	writer := bufio.NewWriter(out)

	for i := 0; i < numSectors; i++ {
		if _, err := io.ReadFull(binFile, buf); err != nil {
			return fmt.Errorf("read sector %d: %w", i, err)
		}

		if strings.HasPrefix(trackType, "MODE1/2352") || strings.HasPrefix(trackType, "MODE2/2352") {
			// Strip 16-byte sync/header, write 2048 bytes of user data
			if _, err := writer.Write(buf[16 : 16+2048]); err != nil {
				return err
			}
		} else {
			// Raw - write full sector
			if _, err := writer.Write(buf); err != nil {
				return err
			}
		}
	}
	return writer.Flush()
}

// writeAudioTrack extracts audio sectors and writes them as a WAV file.
// CD audio is 16-bit stereo 44100Hz little-endian PCM.
func writeAudioTrack(binFile *os.File, outPath string, numSectors int) error {
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	dataSize := numSectors * sectorSize

	// Write WAV header
	if err := writeWAVHeader(out, dataSize); err != nil {
		return err
	}

	// Copy raw audio data (CD audio sectors are already 16-bit stereo PCM)
	buf := make([]byte, sectorSize)
	for i := 0; i < numSectors; i++ {
		if _, err := io.ReadFull(binFile, buf); err != nil {
			return fmt.Errorf("read sector %d: %w", i, err)
		}
		if _, err := out.Write(buf); err != nil {
			return err
		}
	}

	return nil
}

// writeWAVHeader writes a standard WAV file header for CD audio data.
// CD audio: 44100 Hz, 16-bit, stereo, little-endian PCM.
func writeWAVHeader(w io.Writer, dataSize int) error {
	const (
		sampleRate  = 44100
		numChannels = 2
		bitsPerSamp = 16
		byteRate    = sampleRate * numChannels * bitsPerSamp / 8
		blockAlign  = numChannels * bitsPerSamp / 8
	)

	fileSize := 36 + dataSize

	// RIFF header
	if _, err := w.Write([]byte("RIFF")); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(fileSize)); err != nil {
		return err
	}
	if _, err := w.Write([]byte("WAVE")); err != nil {
		return err
	}

	// fmt chunk
	if _, err := w.Write([]byte("fmt ")); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(16)); err != nil { // chunk size
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint16(1)); err != nil { // PCM format
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint16(numChannels)); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(sampleRate)); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(byteRate)); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint16(blockAlign)); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint16(bitsPerSamp)); err != nil {
		return err
	}

	// data chunk
	if _, err := w.Write([]byte("data")); err != nil {
		return err
	}
	return binary.Write(w, binary.LittleEndian, uint32(dataSize))
}
