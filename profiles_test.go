package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

type nullReporter struct{}

func (nullReporter) Log(string)                  {}
func (nullReporter) Logf(string, ...interface{}) {}
func (nullReporter) Step(int, int, string)       {}
func (nullReporter) Error(string)                {}
func (nullReporter) Writer() io.Writer           { return io.Discard }

func TestBuiltinProfilesLoad(t *testing.T) {
	for _, slug := range []string{"civnet", "civ2", "colonization"} {
		p, ok := LookupProfile(slug)
		if !ok {
			t.Fatalf("builtin profile %q not found", slug)
		}
		if p.Exe == "" || p.Name == "" || p.BundleID == "" || p.GameDir == "" {
			t.Errorf("profile %q missing defaults: %+v", slug, p)
		}
		if _, err := parseInstallStrategy(p.Install); err != nil {
			t.Errorf("profile %q: %v", slug, err)
		}
	}
	if errs := ProfileLoadErrors(); len(errs) != 0 {
		t.Errorf("profile load errors: %v", errs)
	}
}

func TestCivNetProfileDetails(t *testing.T) {
	p, ok := LookupProfile("civnet")
	if !ok {
		t.Fatal("civnet profile not found")
	}
	if !p.Win16 {
		t.Error("civnet must be Win16")
	}
	if len(p.Keymap) != 8 {
		t.Errorf("civnet keymap: got %d entries, want 8", len(p.Keymap))
	}
	if len(p.HexPatches) != 1 {
		t.Fatalf("civnet hexpatches: got %d, want 1", len(p.HexPatches))
	}
	h := p.HexPatches[0]
	if h.Offset != 0x147cff || h.ExpectSize != 2073600 || !h.Optional {
		t.Errorf("civnet widescreen patch fields wrong: %+v", h)
	}
	if len(p.Overlays) != 1 || p.Overlays[0].Source != "civnet" {
		t.Errorf("civnet overlay wrong: %+v", p.Overlays)
	}
}

func TestParseInstallStrategy(t *testing.T) {
	cases := []struct {
		in   string
		ok   bool
		kind string
		arg  string
	}{
		{"copy-cd-root", true, "copy-cd-root", ""},
		{"copy-dir:INSTALL", true, "copy-dir", "INSTALL"},
		{"extract-installshield", true, "extract-installshield", ""},
		{"run-installer:INSTALL/SETUP.EXE", true, "run-installer", "INSTALL/SETUP.EXE"},
		{"copy-dir", false, "", ""},      // missing required arg
		{"run-installer", false, "", ""}, // missing required arg
		{"teleport", false, "", ""},      // unknown
	}
	for _, c := range cases {
		s, err := parseInstallStrategy(c.in)
		if c.ok != (err == nil) {
			t.Errorf("parseInstallStrategy(%q) error = %v, want ok=%v", c.in, err, c.ok)
			continue
		}
		if c.ok && (s.Kind != c.kind || s.Arg != c.arg) {
			t.Errorf("parseInstallStrategy(%q) = %+v", c.in, s)
		}
	}
}

func TestApplyHexPatch(t *testing.T) {
	dir := t.TempDir()
	data := make([]byte, 64)
	data[10], data[11], data[12] = 0xAA, 0xBB, 0xCC
	path := filepath.Join(dir, "GAME.EXE")
	if err := os.WriteFile(path, data, 0755); err != nil {
		t.Fatal(err)
	}

	h := HexPatch{
		File:       "game.exe", // case-insensitive match
		ExpectSize: 64,
		Offset:     10,
		Expect:     []int{0xAA, 0xBB, 0xCC},
		Replace:    []int{0x01, 0x02, 0x03},
	}

	if err := applyHexPatch(dir, h, nullReporter{}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, _ := os.ReadFile(path)
	if got[10] != 0x01 || got[11] != 0x02 || got[12] != 0x03 {
		t.Errorf("patch not applied: % x", got[8:16])
	}

	// Idempotent: applying again succeeds without change
	if err := applyHexPatch(dir, h, nullReporter{}); err != nil {
		t.Errorf("re-apply should succeed (already patched): %v", err)
	}

	// Wrong size fails
	h2 := h
	h2.ExpectSize = 99
	if err := applyHexPatch(dir, h2, nullReporter{}); err == nil {
		t.Error("expected size-mismatch error")
	}

	// Byte mismatch fails
	h3 := h
	h3.Expect = []int{0xDE, 0xAD, 0xBF}
	h3.Replace = []int{0x00, 0x00, 0x00}
	if err := applyHexPatch(dir, h3, nullReporter{}); err == nil {
		t.Error("expected byte-mismatch error")
	}
}

func TestCustomProfileDefaults(t *testing.T) {
	p := CustomProfile("MYGAME.EXE")
	if p.Name != "MYGAME" || p.Slug != "mygame" {
		t.Errorf("name/slug: %q/%q", p.Name, p.Slug)
	}
	if p.BundleID != "com.retrowine.mygame" || p.Install != "copy-cd-root" {
		t.Errorf("defaults: %+v", p)
	}
}
