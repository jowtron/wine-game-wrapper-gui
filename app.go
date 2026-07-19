package main

import (
	"context"
	"os"
	"path/filepath"
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

// resolveOutputPath returns the absolute .app path that the build would produce.
func resolveOutputPath(config BuildConfig) string {
	appPath := config.OutputPath
	if appPath == "" {
		name := "Game"
		if config.GameSlug != "" && config.GameSlug != "custom" {
			if p, ok := LookupProfile(config.GameSlug); ok {
				name = p.Name
			}
		}
		appPath = filepath.Join("/Applications", name+".app")
	}
	if !strings.HasSuffix(appPath, ".app") {
		appPath += ".app"
	}
	appPath, _ = filepath.Abs(appPath)
	return appPath
}

// StartBuild launches the build pipeline in a background goroutine.
func (a *App) StartBuild(config BuildConfig) error {
	a.mu.Lock()
	if a.building {
		a.mu.Unlock()
		return nil
	}
	a.mu.Unlock()

	// Check if output already exists and prompt to overwrite
	appPath := resolveOutputPath(config)
	if _, err := os.Stat(appPath); err == nil {
		result, err := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:          runtime.QuestionDialog,
			Title:         "Overwrite Existing App?",
			Message:       appPath + " already exists. Do you want to replace it?",
			DefaultButton: "No",
			CancelButton:  "No",
			Buttons:       []string{"Yes", "No"},
		})
		if err != nil || result != "Yes" {
			return nil
		}
		os.RemoveAll(appPath)
	}

	a.mu.Lock()
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
