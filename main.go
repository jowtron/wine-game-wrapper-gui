package main

import (
	"embed"
	"flag"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend
var assets embed.FS

func main() {
	// Headless mode: `wine-game-wrapper-gui build -game civnet -cue path.cue`
	// runs the pipeline without the GUI (scripting, regression tests).
	if len(os.Args) > 1 && os.Args[1] == "build" {
		os.Exit(runHeadless(os.Args[2:]))
	}

	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "Wine Game Wrapper",
		Width:  800,
		Height: 700,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

// runHeadless parses build flags and runs the pipeline with a stdout reporter.
func runHeadless(args []string) int {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	game := fs.String("game", "", "game profile slug (see -list), or empty with -exe for a custom game")
	exe := fs.String("exe", "", "custom game exe name (when no profile matches)")
	cue := fs.String("cue", "", "path to the CUE file (disc-sourced games)")
	src := fs.String("src", "", "path to an existing install folder (copy-source-dir games, e.g. GOG installs)")
	out := fs.String("o", "", "output .app path (default /Applications/<Name>.app)")
	wine := fs.String("wine", "", "path to a local Wine installation (default: download)")
	otvdm := fs.String("otvdm", "", "path to an otvdm directory (default: embedded)")
	mcicda := fs.String("mcicda", "", "path to mcicda.dll (default: embedded)")
	win16 := fs.Bool("win16", false, "force Win16 mode for custom games")
	overwrite := fs.Bool("overwrite", false, "replace the output .app if it exists")
	list := fs.Bool("list", false, "list available game profiles and exit")
	fs.Parse(args)

	if *list {
		for _, p := range GetAllProfiles() {
			mode := "Win32"
			if p.Win16 {
				mode = "Win16"
			}
			fmt.Printf("%-16s %-24s %-14s %s\n", p.Slug, p.Name, p.Exe, mode)
		}
		for _, e := range ProfileLoadErrors() {
			fmt.Fprintf(os.Stderr, "warning: skipped profile %s\n", e)
		}
		return 0
	}

	if *cue == "" && *src == "" {
		fmt.Fprintln(os.Stderr, "error: -cue (disc image) or -src (install folder) is required")
		fs.Usage()
		return 2
	}
	if *game == "" && *exe == "" {
		fmt.Fprintln(os.Stderr, "error: -game or -exe is required")
		fs.Usage()
		return 2
	}

	config := BuildConfig{
		GameSlug:   *game,
		CustomExe:  *exe,
		CuePath:    *cue,
		SourceDir:  *src,
		OutputPath: *out,
		WinePath:   *wine,
		OtvdmPath:  *otvdm,
		McicdaPath: *mcicda,
		Win16:      *win16,
	}

	if *overwrite {
		appPath := resolveOutputPath(config)
		if _, err := os.Stat(appPath); err == nil {
			fmt.Printf("Removing existing %s\n", appPath)
			if err := os.RemoveAll(appPath); err != nil {
				fmt.Fprintf(os.Stderr, "error: remove existing app: %v\n", err)
				return 1
			}
		}
	}

	if err := runPipeline(config, NewCLIReporter()); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}
