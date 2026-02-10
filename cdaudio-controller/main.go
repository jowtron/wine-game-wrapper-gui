package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Audio formats in order of preference
var audioFormats = []string{".flac", ".wav", ".mp3", ".ogg", ".m4a", ".aac"}

// State
var (
	currentPlayer *exec.Cmd
	currentTrack  int
	isPaused      bool
	musicDir      string
	audioPlayer   []string
)

func detectAudioPlayer() []string {
	var players [][]string

	switch runtime.GOOS {
	case "darwin":
		players = [][]string{
			{"afplay"},
			{"ffplay", "-nodisp", "-autoexit", "-loglevel", "quiet"},
		}
	case "linux":
		players = [][]string{
			{"paplay"},
			{"pw-play"},
			{"ffplay", "-nodisp", "-autoexit", "-loglevel", "quiet"},
			{"mpv", "--no-video", "--really-quiet"},
			{"aplay"}, // WAV only
		}
	default:
		players = [][]string{
			{"ffplay", "-nodisp", "-autoexit", "-loglevel", "quiet"},
		}
	}

	for _, player := range players {
		if _, err := exec.LookPath(player[0]); err == nil {
			return player
		}
	}

	return nil
}

func getTrackFile(trackNum int) string {
	for _, ext := range audioFormats {
		trackFile := filepath.Join(musicDir, fmt.Sprintf("track%02d%s", trackNum, ext))
		if _, err := os.Stat(trackFile); err == nil {
			return trackFile
		}
	}
	return ""
}

func playTrack(trackNum int) bool {
	stopPlayback()

	if audioPlayer == nil {
		fmt.Println("No audio player available")
		return false
	}

	trackFile := getTrackFile(trackNum)
	if trackFile == "" {
		fmt.Printf("Track %d not found\n", trackNum)
		return false
	}

	fmt.Printf("Playing track %d: %s\n", trackNum, filepath.Base(trackFile))
	currentTrack = trackNum
	isPaused = false

	args := append(audioPlayer[1:], trackFile)
	currentPlayer = exec.Command(audioPlayer[0], args...)
	currentPlayer.Stdout = nil
	currentPlayer.Stderr = nil

	if err := currentPlayer.Start(); err != nil {
		fmt.Printf("Error starting player: %v\n", err)
		return false
	}

	return true
}

func stopPlayback() {
	if currentPlayer != nil && currentPlayer.Process != nil {
		currentPlayer.Process.Signal(syscall.SIGTERM)
		done := make(chan error, 1)
		go func() {
			done <- currentPlayer.Wait()
		}()

		select {
		case <-done:
		case <-time.After(time.Second):
			currentPlayer.Process.Kill()
		}
		currentPlayer = nil
	}
	isPaused = false
}

func pausePlayback() {
	if currentPlayer != nil && currentPlayer.Process != nil && !isPaused {
		if runtime.GOOS != "windows" {
			currentPlayer.Process.Signal(syscall.SIGSTOP)
			isPaused = true
			fmt.Println("Paused")
		}
	}
}

func resumePlayback() {
	if currentPlayer != nil && currentPlayer.Process != nil && isPaused {
		if runtime.GOOS != "windows" {
			currentPlayer.Process.Signal(syscall.SIGCONT)
			isPaused = false
			fmt.Println("Resumed")
		}
	}
}

func processCommand(cmd string) {
	parts := strings.Fields(strings.TrimSpace(cmd))
	if len(parts) == 0 {
		return
	}

	action := strings.ToUpper(parts[0])

	switch action {
	case "PLAY":
		trackFrom := 2
		if len(parts) > 1 {
			if n, err := strconv.Atoi(parts[1]); err == nil {
				trackFrom = n
			}
		}
		playTrack(trackFrom)

	case "STOP":
		fmt.Println("Stop")
		stopPlayback()

	case "PAUSE":
		pausePlayback()

	case "RESUME":
		resumePlayback()

	case "SEEK":
		track := 2
		if len(parts) > 1 {
			if n, err := strconv.Atoi(parts[1]); err == nil {
				track = n
			}
		}
		fmt.Printf("Seek to track %d\n", track)
		currentTrack = track

	case "OPEN":
		fmt.Println("CD Audio device opened")

	case "CLOSE":
		fmt.Println("CD Audio device closed")
		stopPlayback()
	}
}

func watchLog(logFile string) error {
	// Create log file if it doesn't exist
	dir := filepath.Dir(logFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	f, err := os.OpenFile(logFile, os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer f.Close()

	// Seek to end
	f.Seek(0, io.SeekEnd)

	fmt.Printf("Watching: %s\n", logFile)
	fmt.Println("Waiting for CD audio commands...")
	fmt.Println("----------------------------------------")

	reader := bufio.NewReader(f)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				return err
			}
			// Check if player finished
			if currentPlayer != nil && currentPlayer.ProcessState != nil {
				// Track ended
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		processCommand(line)
	}
}

func countTracks() int {
	count := 0
	for _, ext := range audioFormats {
		matches, _ := filepath.Glob(filepath.Join(musicDir, "track*"+ext))
		count += len(matches)
	}
	return count
}

func main() {
	// Parse flags
	musicDirFlag := flag.String("music-dir", "", "Directory containing audio tracks")
	logFileFlag := flag.String("log-file", "", "Path to mcicda_commands.log")
	winePrefixFlag := flag.String("wine-prefix", "", "Wine prefix path")
	flag.Parse()

	// Determine music directory
	execDir, _ := os.Executable()
	execDir = filepath.Dir(execDir)

	if *musicDirFlag != "" {
		musicDir = *musicDirFlag
	} else if _, err := os.Stat(filepath.Join(execDir, "music")); err == nil {
		musicDir = filepath.Join(execDir, "music")
	} else if _, err := os.Stat(filepath.Join(execDir, "virtual_cdrom")); err == nil {
		musicDir = filepath.Join(execDir, "virtual_cdrom")
	} else {
		musicDir = filepath.Join(execDir, "music")
	}

	// Determine log file
	var logFile string
	if *logFileFlag != "" {
		logFile = *logFileFlag
	} else if *winePrefixFlag != "" {
		logFile = filepath.Join(*winePrefixFlag, "drive_c", "mcicda_commands.log")
	} else if winePrefix := os.Getenv("WINEPREFIX"); winePrefix != "" {
		logFile = filepath.Join(winePrefix, "drive_c", "mcicda_commands.log")
	} else if _, err := os.Stat(filepath.Join(execDir, "wine-prefix")); err == nil {
		logFile = filepath.Join(execDir, "wine-prefix", "drive_c", "mcicda_commands.log")
	} else {
		home, _ := os.UserHomeDir()
		logFile = filepath.Join(home, ".wine", "drive_c", "mcicda_commands.log")
	}

	// Detect audio player
	audioPlayer = detectAudioPlayer()

	fmt.Println("========================================")
	fmt.Println("CD Audio Controller")
	fmt.Println("========================================")
	fmt.Printf("Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	if audioPlayer != nil {
		fmt.Printf("Audio player: %s\n", audioPlayer[0])
	} else {
		fmt.Println("Audio player: None (install ffmpeg, mpv, or pulseaudio)")
	}
	fmt.Printf("Music directory: %s\n", musicDir)
	fmt.Printf("Log file: %s\n", logFile)
	fmt.Println()

	// Count tracks
	trackCount := countTracks()
	if trackCount == 0 {
		fmt.Println("WARNING: No audio track files found in music directory!")
		fmt.Println("Expected files like: track02.flac, track03.wav, etc.")
	} else {
		fmt.Printf("Found %d audio tracks\n", trackCount)
	}
	fmt.Println()

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		stopPlayback()
		os.Exit(0)
	}()

	// Watch log
	if err := watchLog(logFile); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
