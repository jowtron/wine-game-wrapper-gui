package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

func TestARCVDecompress(t *testing.T) {
	data, err := os.ReadFile("testdata/comt.arcv")
	if err != nil {
		t.Fatal(err)
	}
	if !IsARCV(data) {
		t.Fatal("fixture not detected as ARCV")
	}
	f, err := ParseARCV(data)
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "comt.wri" {
		t.Errorf("name = %q, want comt.wri", f.Name)
	}
	if f.OrigSize != 3072 {
		t.Errorf("orig size = %d, want 3072", f.OrigSize)
	}
	if f.CRC != 0xe8210a9a {
		t.Errorf("crc = %08x, want e8210a9a", f.CRC)
	}

	out, err := f.Decompress()
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3072 {
		t.Fatalf("decompressed %d bytes, want 3072", len(out))
	}
	// Known-good output hash (verified against header CRC and by the file
	// being a valid, readable Microsoft Write document)
	sum := sha256.Sum256(out)
	const want = "edfc2d3097fe1c0a1a7b165af5305dc51b5edb033a041b9fdf78b26dca9d3bd6"
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Errorf("output sha256 = %s, want %s", got, want)
	}
	// Output must start with the Write 3.0 magic
	if out[0] != 0x31 || out[1] != 0xbe {
		t.Errorf("output does not start with WRI magic: % x", out[:4])
	}
}

func TestARCVCorruptData(t *testing.T) {
	data, err := os.ReadFile("testdata/comt.arcv")
	if err != nil {
		t.Fatal(err)
	}
	// Flip a bit in the compressed stream: CRC check must catch it
	corrupt := make([]byte, len(data))
	copy(corrupt, data)
	corrupt[len(corrupt)-10] ^= 0x01
	f, err := ParseARCV(corrupt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Decompress(); err == nil {
		t.Error("expected CRC error on corrupt data")
	}
}

func TestIsARCVNegative(t *testing.T) {
	for _, d := range [][]byte{
		nil,
		[]byte("MZ this is not an archive, just some bytes here"),
		[]byte("ARCV"), // too short
	} {
		if IsARCV(d) {
			t.Errorf("IsARCV(%q) = true, want false", d)
		}
	}
}
