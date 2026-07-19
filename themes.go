package main

import "strings"

// Windows-appearance theming.
//
// Wine does not observe the macOS light/dark setting, but the classic apps
// these bundles run draw their menus, dialogs, and window chrome from
// Windows' system color palette (GetSysColor). Wine exposes that palette as
// registry strings under HKCU\Control Panel\Colors, so we can ship a coherent
// dark (or light) Windows theme in the prefix.
//
// This themes the Windows UI chrome only — menus, dialogs, buttons, scrollbars,
// title bars, tooltips — not a game's own rendered bitmap content (map/city
// screens stay as drawn). Colors are read at process start, so we apply before
// launching the game; the theme does not follow a mid-session macOS toggle.

// themeColors maps a mode ("dark"/"light") to the HKCU\Control Panel\Colors
// values. Each value is a Windows "R G B" system color string.
var themeColors = map[string]map[string]string{
	"dark": {
		"ActiveBorder":          "43 43 43",
		"ActiveTitle":           "45 45 45",
		"AppWorkSpace":          "30 30 30",
		"Background":            "0 0 0",
		"ButtonAlternateFace":   "53 53 53",
		"ButtonDkShadow":        "0 0 0",
		"ButtonFace":            "53 53 53",
		"ButtonHilight":         "90 90 90",
		"ButtonLight":           "70 70 70",
		"ButtonShadow":          "30 30 30",
		"ButtonText":            "245 245 245",
		"GradientActiveTitle":   "45 45 45",
		"GradientInactiveTitle": "30 30 30",
		"GrayText":              "140 140 140",
		"Hilight":               "38 79 120",
		"HilightText":           "255 255 255",
		"HotTrackingColor":      "80 140 200",
		"InactiveBorder":        "30 30 30",
		"InactiveTitle":         "30 30 30",
		"InactiveTitleText":     "160 160 160",
		"InfoText":              "245 245 245",
		"InfoWindow":            "53 53 53",
		"Menu":                  "43 43 43",
		"MenuBar":               "43 43 43",
		"MenuHilight":           "38 79 120",
		"MenuText":              "245 245 245",
		"Scrollbar":             "43 43 43",
		"TitleText":             "255 255 255",
		"Window":                "32 32 32",
		"WindowFrame":           "20 20 20",
		"WindowText":            "245 245 245",
	},
	// Classic Windows light palette (Wine's defaults), written explicitly so
	// switching auto -> light is deterministic and reverses a dark apply.
	"light": {
		"ActiveBorder":          "212 208 200",
		"ActiveTitle":           "10 36 106",
		"AppWorkSpace":          "128 128 128",
		"Background":            "58 110 165",
		"ButtonAlternateFace":   "181 181 181",
		"ButtonDkShadow":        "64 64 64",
		"ButtonFace":            "212 208 200",
		"ButtonHilight":         "255 255 255",
		"ButtonLight":           "212 208 200",
		"ButtonShadow":          "128 128 128",
		"ButtonText":            "0 0 0",
		"GradientActiveTitle":   "166 202 240",
		"GradientInactiveTitle": "192 192 192",
		"GrayText":              "128 128 128",
		"Hilight":               "10 36 106",
		"HilightText":           "255 255 255",
		"HotTrackingColor":      "0 0 128",
		"InactiveBorder":        "212 208 200",
		"InactiveTitle":         "128 128 128",
		"InactiveTitleText":     "212 208 200",
		"InfoText":              "0 0 0",
		"InfoWindow":            "255 255 225",
		"Menu":                  "212 208 200",
		"MenuBar":               "212 208 200",
		"MenuHilight":           "10 36 106",
		"MenuText":              "0 0 0",
		"Scrollbar":             "212 208 200",
		"TitleText":             "255 255 255",
		"WindowFrame":           "0 0 0",
		"Window":                "255 255 255",
		"WindowText":            "0 0 0",
	},
}

// themeColorsReg returns a REGEDIT4 document setting the color palette for the
// given mode ("dark" or "light"). Returns "" for an unknown mode.
func themeColorsReg(mode string) string {
	colors, ok := themeColors[mode]
	if !ok {
		return ""
	}
	// Deterministic key order for reproducible builds/tests.
	keys := make([]string, 0, len(colors))
	for k := range colors {
		keys = append(keys, k)
	}
	// simple insertion sort to avoid importing sort in a tiny file
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}

	var b strings.Builder
	b.WriteString("REGEDIT4\r\n\r\n")
	b.WriteString("[HKEY_CURRENT_USER\\Control Panel\\Colors]\r\n")
	for _, k := range keys {
		b.WriteString("\"")
		b.WriteString(k)
		b.WriteString("\"=\"")
		b.WriteString(colors[k])
		b.WriteString("\"\r\n")
	}
	return b.String()
}
