package main

import (
	"context"
	"os"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct holds application state and is bound to the frontend.
type App struct {
	ctx      context.Context
	building bool
	mu       sync.Mutex
}

// NewApp creates a new App instance.
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the Wails runtime.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// GetProfiles returns all built-in game profiles for the frontend dropdown.
func (a *App) GetProfiles() []ProfileInfo {
	return GetAllProfiles()
}

// SelectCUEFile opens a native file dialog filtered to *.cue files.
func (a *App) SelectCUEFile() (string, error) {
	home, _ := os.UserHomeDir()
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "Select CUE File",
		DefaultDirectory: home,
		Filters: []runtime.FileFilter{
			{
				DisplayName: "CUE Files (*.cue)",
				Pattern:     "*.cue",
			},
		},
	})
	return path, err
}

// SelectDirectory opens a native directory picker.
func (a *App) SelectDirectory(title string) (string, error) {
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: title,
	})
	return path, err
}

// SelectFile opens a native file picker.
func (a *App) SelectFile(title string) (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: title,
	})
	return path, err
}

// SelectOutputPath opens a save dialog for the .app output.
func (a *App) SelectOutputPath(gameName string) (string, error) {
	defaultName := "Game.app"
	if gameName != "" {
		defaultName = gameName + ".app"
	}
	appsDir := "/Applications"
	if _, err := os.Stat(appsDir); err != nil {
		appsDir, _ = os.UserHomeDir()
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:            "Save .app Bundle As",
		DefaultDirectory: appsDir,
		DefaultFilename:  defaultName,
	})
	if err != nil {
		return "", err
	}
	// macOS save dialog may strip .app extension
	if path != "" && !strings.HasSuffix(path, ".app") {
		path += ".app"
	}
	return path, nil
}

// IsBuilding returns whether a build is currently in progress.
func (a *App) IsBuilding() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.building
}

// StartBuild launches the build pipeline in a background goroutine.
func (a *App) StartBuild(config BuildConfig) error {
	a.mu.Lock()
	if a.building {
		a.mu.Unlock()
		return nil
	}
	a.building = true
	a.mu.Unlock()

	reporter := NewWailsReporter(a.ctx)

	go func() {
		defer func() {
			a.mu.Lock()
			a.building = false
			a.mu.Unlock()
		}()

		err := runPipeline(config, reporter)
		if err != nil {
			reporter.Error(err.Error())
			runtime.EventsEmit(a.ctx, "build:complete", map[string]interface{}{
				"success": false,
				"error":   err.Error(),
			})
			return
		}

		runtime.EventsEmit(a.ctx, "build:complete", map[string]interface{}{
			"success": true,
		})
	}()

	return nil
}
