package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

// GameProfile defines a game's configuration for building a .app wrapper.
// Profiles are declarative TOML documents: builtins are embedded under
// resources/profiles/, and users can drop additional .toml files into
// ~/Library/Application Support/wine-game-wrapper/profiles/ without recompiling.
type GameProfile struct {
	Name     string   `toml:"name"`      // Display name, e.g. "CivNet"
	Slug     string   `toml:"slug"`      // Short identifier; default: lowercased name
	Exe      string   `toml:"exe"`       // Main executable, e.g. "CIVNET.EXE"
	Win16    bool     `toml:"win16"`     // True if Win16 app requiring otvdm
	BundleID string   `toml:"bundle_id"` // macOS bundle identifier; default com.retrowine.<slug>
	GameDir  string   `toml:"game_dir"`  // Directory name inside drive_c; default: name
	Desktop  string   `toml:"desktop"`   // Wine virtual desktop size; default 1920x1080
	Theme    string   `toml:"theme"`     // Windows UI theme: "", "light", "dark", "auto"
	Install  string   `toml:"install"`   // Install strategy; default copy-cd-root (see install.go)
	RetainCD []string `toml:"retain_cd"` // CD paths to bundle and map as drive d:
	CDLabel  string   `toml:"cd_label"`  // Volume label for the emulated d: drive

	Overlays   []OverlayPatch `toml:"overlay"`  // File-copy patch sets
	HexPatches []HexPatch     `toml:"hexpatch"` // In-place byte patches
	Keymap     []KeymapEntry  `toml:"keymap"`   // Keyboard remappings by key name
}

// OverlayPatch copies the files of resources/patches/<Source> over the
// installed game directory (case-insensitive filename matching).
type OverlayPatch struct {
	Source string   `toml:"source"`
	Skip   []string `toml:"skip"` // Filenames to skip (e.g. readme files)
}

// HexPatch is an in-place byte patch with verification, e.g. a widescreen fix.
type HexPatch struct {
	File       string `toml:"file"`        // Target file inside the game dir
	Desc       string `toml:"desc"`        // Human-readable description for logs
	ExpectSize int64  `toml:"expect_size"` // Exact file size guard (0 = don't check)
	Offset     int64  `toml:"offset"`
	Expect     []int  `toml:"expect"`   // Bytes that must be present at offset
	Replace    []int  `toml:"replace"`  // Bytes to write at offset
	Optional   bool   `toml:"optional"` // Log-and-continue instead of failing the build
}

// KeymapEntry remaps one key to another by name (see keyTable).
type KeymapEntry struct {
	From string `toml:"from"`
	To   string `toml:"to"`
}

// keyInfo holds the PC set-1 scancode and Windows virtual-key code for a key.
type keyInfo struct {
	Scancode byte
	VK       byte
}

// keyTable maps profile key names to scancode + VK pairs.
// This is the single source of truth for key remapping (the launcher's
// keyremap.ini needs both codes).
var keyTable = map[string]keyInfo{
	"1": {0x02, 0x31}, "2": {0x03, 0x32}, "3": {0x04, 0x33}, "4": {0x05, 0x34},
	"5": {0x06, 0x35}, "6": {0x07, 0x36}, "7": {0x08, 0x37}, "8": {0x09, 0x38},
	"9": {0x0A, 0x39}, "0": {0x0B, 0x30},

	"q": {0x10, 0x51}, "w": {0x11, 0x57}, "e": {0x12, 0x45}, "r": {0x13, 0x52},
	"t": {0x14, 0x54}, "y": {0x15, 0x59}, "u": {0x16, 0x55}, "i": {0x17, 0x49},
	"o": {0x18, 0x4F}, "p": {0x19, 0x50},

	"a": {0x1E, 0x41}, "s": {0x1F, 0x53}, "d": {0x20, 0x44}, "f": {0x21, 0x46},
	"g": {0x22, 0x47}, "h": {0x23, 0x48}, "j": {0x24, 0x4A}, "k": {0x25, 0x4B},
	"l": {0x26, 0x4C},

	"z": {0x2C, 0x5A}, "x": {0x2D, 0x58}, "c": {0x2E, 0x43}, "v": {0x2F, 0x56},
	"b": {0x30, 0x42}, "n": {0x31, 0x4E}, "m": {0x32, 0x4D},

	"num0": {0x52, 0x60}, "num1": {0x4F, 0x61}, "num2": {0x50, 0x62},
	"num3": {0x51, 0x63}, "num4": {0x4B, 0x64}, "num5": {0x4C, 0x65},
	"num6": {0x4D, 0x66}, "num7": {0x47, 0x67}, "num8": {0x48, 0x68},
	"num9": {0x49, 0x69},
}

// LookupKey resolves a profile key name to its scancode and VK code.
func LookupKey(name string) (keyInfo, bool) {
	k, ok := keyTable[strings.ToLower(name)]
	return k, ok
}

// applyDefaults fills optional profile fields and validates required ones.
func (p *GameProfile) applyDefaults() error {
	if p.Name == "" {
		return fmt.Errorf("profile is missing 'name'")
	}
	if p.Exe == "" {
		return fmt.Errorf("profile %q is missing 'exe'", p.Name)
	}
	if p.Slug == "" {
		p.Slug = strings.ToLower(strings.ReplaceAll(p.Name, " ", "-"))
	}
	p.Slug = strings.ToLower(p.Slug)
	if p.GameDir == "" {
		p.GameDir = p.Name
	}
	if p.BundleID == "" {
		p.BundleID = "com.retrowine." + p.Slug
	}
	if p.Desktop == "" {
		p.Desktop = "1920x1080"
	}
	if p.Install == "" {
		p.Install = "copy-cd-root"
	}
	if _, err := parseInstallStrategy(p.Install); err != nil {
		return fmt.Errorf("profile %q: %w", p.Name, err)
	}
	switch p.Theme {
	case "", "light", "dark", "auto":
	default:
		return fmt.Errorf("profile %q: theme must be light, dark, or auto (got %q)", p.Name, p.Theme)
	}
	for _, h := range p.HexPatches {
		if h.File == "" || len(h.Expect) == 0 || len(h.Expect) != len(h.Replace) {
			return fmt.Errorf("profile %q: hexpatch needs 'file' and equal-length 'expect'/'replace'", p.Name)
		}
		for _, b := range append(append([]int{}, h.Expect...), h.Replace...) {
			if b < 0 || b > 255 {
				return fmt.Errorf("profile %q: hexpatch byte %d out of range", p.Name, b)
			}
		}
	}
	for _, k := range p.Keymap {
		if _, ok := LookupKey(k.From); !ok {
			return fmt.Errorf("profile %q: unknown key name %q", p.Name, k.From)
		}
		if _, ok := LookupKey(k.To); !ok {
			return fmt.Errorf("profile %q: unknown key name %q", p.Name, k.To)
		}
	}
	return nil
}

// userProfilesDir is where users can drop additional profile .toml files.
func userProfilesDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support", "wine-game-wrapper", "profiles")
}

var (
	profileRegistry map[string]GameProfile
	profileOrder    []string // slugs, builtins-then-user, alphabetical within each
	profileOnce     sync.Once
	profileLoadErrs []string
)

// loadProfiles parses embedded builtin profiles plus any user profiles.
// User profiles with the same slug override builtins. Parse errors don't
// abort loading; they are collected into profileLoadErrs.
func loadProfiles() {
	profileRegistry = map[string]GameProfile{}

	addProfile := func(data []byte, origin string) {
		var p GameProfile
		if err := toml.Unmarshal(data, &p); err != nil {
			profileLoadErrs = append(profileLoadErrs, fmt.Sprintf("%s: %v", origin, err))
			return
		}
		if err := p.applyDefaults(); err != nil {
			profileLoadErrs = append(profileLoadErrs, fmt.Sprintf("%s: %v", origin, err))
			return
		}
		if _, exists := profileRegistry[p.Slug]; !exists {
			profileOrder = append(profileOrder, p.Slug)
		}
		profileRegistry[p.Slug] = p
	}

	// Builtins from the embedded resources FS
	var builtinNames []string
	entries, err := fs.ReadDir(embeddedResources, "resources/profiles")
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".toml") {
				builtinNames = append(builtinNames, e.Name())
			}
		}
	}
	sort.Strings(builtinNames)
	for _, name := range builtinNames {
		data, err := embeddedResources.ReadFile("resources/profiles/" + name)
		if err != nil {
			profileLoadErrs = append(profileLoadErrs, fmt.Sprintf("embedded %s: %v", name, err))
			continue
		}
		addProfile(data, "builtin "+name)
	}

	// User drop-in profiles
	if dir := userProfilesDir(); dir != "" {
		entries, err := os.ReadDir(dir)
		if err == nil {
			var names []string
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".toml") {
					names = append(names, e.Name())
				}
			}
			sort.Strings(names)
			for _, name := range names {
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil {
					profileLoadErrs = append(profileLoadErrs, fmt.Sprintf("%s: %v", name, err))
					continue
				}
				addProfile(data, "user "+name)
			}
		}
	}
}

// LookupProfile returns a game profile by slug (case-insensitive).
func LookupProfile(slug string) (GameProfile, bool) {
	profileOnce.Do(loadProfiles)
	p, ok := profileRegistry[strings.ToLower(slug)]
	return p, ok
}

// ProfileLoadErrors returns any profile files that failed to parse.
func ProfileLoadErrors() []string {
	profileOnce.Do(loadProfiles)
	return profileLoadErrs
}

// CustomProfile creates a profile for an unlisted game.
func CustomProfile(exe string) GameProfile {
	name := strings.TrimSuffix(exe, filepath.Ext(exe))
	p := GameProfile{
		Name: name,
		Slug: strings.ToLower(name),
		Exe:  exe,
	}
	p.applyDefaults()
	return p
}

// ProfileInfo is a simplified profile struct for the frontend dropdown.
type ProfileInfo struct {
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Exe   string `json:"exe"`
	Win16 bool   `json:"win16"`
}

// GetAllProfiles returns all loaded profiles for the frontend, derived from
// the registry so the list can never drift from the actual profiles.
func GetAllProfiles() []ProfileInfo {
	profileOnce.Do(loadProfiles)
	infos := make([]ProfileInfo, 0, len(profileOrder))
	for _, slug := range profileOrder {
		p := profileRegistry[slug]
		infos = append(infos, ProfileInfo{Slug: p.Slug, Name: p.Name, Exe: p.Exe, Win16: p.Win16})
	}
	return infos
}
