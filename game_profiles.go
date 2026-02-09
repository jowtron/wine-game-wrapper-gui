package main

// GameProfile defines a known game's configuration for building a .app wrapper.
type GameProfile struct {
	Name        string       // Display name, e.g. "CivNet"
	Slug        string       // Short identifier, e.g. "civnet"
	Exe         string       // Main executable, e.g. "CIVNET.EXE"
	Win16       bool         // True if Win16 app requiring otvdm
	BundleID    string       // macOS bundle identifier
	GameDir     string       // Directory name inside drive_c, defaults to Name
	ScancodeMap []ScancodeEntry // Key remappings via Windows Scancode Map registry
}

// ScancodeEntry maps one keyboard scancode to another.
// Used to remap regular keys to numpad keys for games that use numpad for movement.
type ScancodeEntry struct {
	From byte // Source scancode (key to remap)
	To   byte // Target scancode (what it becomes)
}

var builtinProfiles = map[string]GameProfile{
	"civnet": {
		Name:     "CivNet",
		Slug:     "civnet",
		Exe:      "CIVNET.EXE",
		Win16:    true,
		BundleID: "com.retrowine.civnet",
		GameDir:  "CivNet",
		// Remap 789/uo/jkl to numpad for unit movement (I kept for irrigate)
		ScancodeMap: []ScancodeEntry{
			{0x08, 0x47}, // 7 → Numpad 7
			{0x09, 0x48}, // 8 → Numpad 8
			{0x0A, 0x49}, // 9 → Numpad 9
			{0x16, 0x4B}, // U → Numpad 4
			{0x18, 0x4D}, // O → Numpad 6
			{0x24, 0x4F}, // J → Numpad 1
			{0x25, 0x50}, // K → Numpad 2
			{0x26, 0x51}, // L → Numpad 3
		},
	},
	"civ2": {
		Name:     "Civilization II",
		Slug:     "civ2",
		Exe:      "civ2.exe",
		Win16:    false,
		BundleID: "com.retrowine.civ2",
		GameDir:  "Civ2",
	},
	"colonization": {
		Name:     "Colonization",
		Slug:     "colonization",
		Exe:      "COLONIZE.EXE",
		Win16:    true,
		BundleID: "com.retrowine.colonization",
		GameDir:  "Colonization",
	},
}

// LookupProfile returns a game profile by slug (case-insensitive).
func LookupProfile(name string) (GameProfile, bool) {
	// Normalize to lowercase
	lower := ""
	for _, c := range name {
		if c >= 'A' && c <= 'Z' {
			lower += string(c + 32)
		} else {
			lower += string(c)
		}
	}
	p, ok := builtinProfiles[lower]
	return p, ok
}

// CustomProfile creates a profile for an unlisted game.
func CustomProfile(exe string) GameProfile {
	// Derive name from exe: "GAME.EXE" -> "GAME"
	name := exe
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			name = name[:i]
			break
		}
	}
	// Title case the first letter
	slug := ""
	for _, c := range name {
		if c >= 'A' && c <= 'Z' {
			slug += string(c + 32)
		} else {
			slug += string(c)
		}
	}

	return GameProfile{
		Name:     name,
		Slug:     slug,
		Exe:      exe,
		Win16:    false,
		BundleID: "com.retrowine." + slug,
		GameDir:  name,
	}
}

// ProfileInfo is a simplified profile struct for the frontend dropdown.
type ProfileInfo struct {
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Exe   string `json:"exe"`
	Win16 bool   `json:"win16"`
}

// GetAllProfiles returns all built-in profiles as ProfileInfo for the frontend.
func GetAllProfiles() []ProfileInfo {
	profiles := []ProfileInfo{
		{Slug: "civnet", Name: "CivNet", Exe: "CIVNET.EXE", Win16: true},
		{Slug: "civ2", Name: "Civilization II", Exe: "civ2.exe", Win16: false},
		{Slug: "colonization", Name: "Colonization", Exe: "COLONIZE.EXE", Win16: true},
	}
	return profiles
}
