package main

import "testing"

func TestThemeColorsReg(t *testing.T) {
	for _, mode := range []string{"dark", "light"} {
		reg := themeColorsReg(mode)
		if reg == "" {
			t.Fatalf("themeColorsReg(%q) returned empty", mode)
		}
		for _, want := range []string{
			"REGEDIT4",
			`[HKEY_CURRENT_USER\Control Panel\Colors]`,
			`"Window"=`,
			`"Menu"=`,
			`"WindowText"=`,
		} {
			if !contains(reg, want) {
				t.Errorf("%s theme missing %q", mode, want)
			}
		}
	}
	if themeColorsReg("chartreuse") != "" {
		t.Error("unknown theme should return empty")
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
