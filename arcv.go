package main

// ARCV ("EDI Install archive") decompression.
//
// ARCV is the compressed-file format of EDI Install Pro (Eschalon Development
// / Robert Salesas), the installer used by MicroProse's Windows CD games of
// the mid-90s (e.g. Colonization's COLONIZE.$00). No public decompressor or
// format documentation existed; this implementation was reverse-engineered
// from the 16-bit decompressor in Colonization's SETUP.EXE (2026-07).
//
// The compression is a modified Okumura LZHUF (LZSS + adaptive Huffman):
//   - Alphabet of 287 symbols (T=573): 0-255 literals, 256 = end-of-stream,
//     257-286 = match lengths 3-32 (F=32, vs LZHUF's 60)
//   - 4096-byte window initialized to 0x20; the write position starts at
//     N-T = 3523 — the original code reuses the tree-size constant where
//     LZHUF uses N-F, which is why standard decoders never matched
//   - Match positions use LZHUF's static d_code table but a modified d_len
//     table (runs 3x32 4x48 5x64 6x48 7x48 8x16)
//   - Bits are consumed MSB-first
//   - The file header stores a JAMCRC (CRC-32 without the final inversion)
//     of the *compressed* chunk data
//
// Format layout:
//   File header ("ARCV", version 0x0110):
//     +0  "ARCV"; +4 version u16le; +6 header length u16le; +8 flags u32le
//     +12 name length u8, name bytes; then u32le original size, u32le
//     compressed size, u32le DOS attributes, u32le DOS date/time, two u32le
//     file versions, u32le JAMCRC of compressed data; header ends with a
//     u32le chunk offset at (header length - 4)
//   Chunk ("CHNK"): +4 version u16le; +6 header length u16le (16);
//     +8 flags u32le; +12 data length u32le; then compressed data.

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

const (
	arcvN       = 4096            // LZ window size
	arcvNChar   = 287             // 256 literals + EOF + 30 length codes
	arcvT       = arcvNChar*2 - 1 // 573 tree nodes
	arcvR       = arcvT - 1       // root index
	arcvMaxFreq = 0x8000
)

// arcvDCode is LZHUF's standard d_code table: upper 6 bits of match positions.
var arcvDCode = buildARCVDCode()

func buildARCVDCode() []byte {
	runs := []struct{ count, val int }{
		{32, 0}, {16, 1}, {16, 2}, {16, 3},
	}
	for v := 4; v < 12; v++ {
		runs = append(runs, struct{ count, val int }{8, v})
	}
	for v := 12; v < 24; v++ {
		runs = append(runs, struct{ count, val int }{4, v})
	}
	for v := 24; v < 48; v++ {
		runs = append(runs, struct{ count, val int }{2, v})
	}
	for v := 48; v < 64; v++ {
		runs = append(runs, struct{ count, val int }{1, v})
	}
	t := make([]byte, 0, 256)
	for _, r := range runs {
		for i := 0; i < r.count; i++ {
			t = append(t, byte(r.val))
		}
	}
	return t
}

// arcvDLen is EDI's modified d_len table (code length per position prefix).
// Differs from LZHUF's in the tail: 7x48+8x16 instead of 7x32+8x32.
var arcvDLen = buildARCVDLen()

func buildARCVDLen() []byte {
	t := make([]byte, 0, 256)
	for _, r := range []struct{ count, val int }{
		{32, 3}, {48, 4}, {64, 5}, {48, 6}, {48, 7}, {16, 8},
	} {
		for i := 0; i < r.count; i++ {
			t = append(t, byte(r.val))
		}
	}
	return t
}

// ARCVFile is a parsed ARCV archive (always holds exactly one member file).
type ARCVFile struct {
	Name     string // original filename, e.g. "colonize.exe"
	OrigSize uint32 // decompressed size
	CRC      uint32 // JAMCRC of the compressed data
	stream   []byte // compressed chunk data
}

// IsARCV reports whether data looks like an ARCV v1 archive.
func IsARCV(data []byte) bool {
	return len(data) > 16 && string(data[0:4]) == "ARCV" &&
		data[5] == 0x01 // version high byte; v2 ("Eschalon Setup") differs
}

// ParseARCV parses an ARCV archive header and locates the compressed stream.
func ParseARCV(data []byte) (*ARCVFile, error) {
	if !IsARCV(data) {
		return nil, fmt.Errorf("not an ARCV v1 file")
	}
	hdrLen := int(binary.LittleEndian.Uint16(data[6:8]))
	if hdrLen < 16 || hdrLen+16 > len(data) {
		return nil, fmt.Errorf("invalid ARCV header length %d", hdrLen)
	}

	p := 12
	nameLen := int(data[p])
	p++
	if p+nameLen+32 > len(data) {
		return nil, fmt.Errorf("truncated ARCV header")
	}
	f := &ARCVFile{Name: string(data[p : p+nameLen])}
	p += nameLen
	f.OrigSize = binary.LittleEndian.Uint32(data[p:])
	compSize := binary.LittleEndian.Uint32(data[p+4:])
	// +8 attributes, +12 DOS date/time, +16/+20 file versions
	f.CRC = binary.LittleEndian.Uint32(data[p+24:])

	chunkOff := int(binary.LittleEndian.Uint32(data[hdrLen-4:]))
	if chunkOff+16 > len(data) || string(data[chunkOff:chunkOff+4]) != "CHNK" {
		return nil, fmt.Errorf("ARCV chunk not found at offset %d", chunkOff)
	}
	chunkHdrLen := int(binary.LittleEndian.Uint16(data[chunkOff+6:]))
	dataLen := int(binary.LittleEndian.Uint32(data[chunkOff+12:]))
	if dataLen != int(compSize) {
		return nil, fmt.Errorf("ARCV size mismatch: header says %d, chunk says %d", compSize, dataLen)
	}
	start := chunkOff + chunkHdrLen
	if start+dataLen > len(data) {
		return nil, fmt.Errorf("truncated ARCV data")
	}
	f.stream = data[start : start+dataLen]
	return f, nil
}

// arcvDecoder is the adaptive-Huffman state machine.
type arcvDecoder struct {
	data   []byte
	pos    int
	bitbuf byte
	bitcnt int

	freq [arcvT + 1]uint16
	prnt [arcvT + arcvNChar]int16
	son  [arcvT]int16
}

func newARCVDecoder(stream []byte) *arcvDecoder {
	d := &arcvDecoder{data: stream}
	for i := 0; i < arcvNChar; i++ {
		d.freq[i] = 1
		d.son[i] = int16(i + arcvT)
		d.prnt[i+arcvT] = int16(i)
	}
	i, j := 0, arcvNChar
	for j <= arcvR {
		d.freq[j] = d.freq[i] + d.freq[i+1]
		d.son[j] = int16(i)
		d.prnt[i] = int16(j)
		d.prnt[i+1] = int16(j)
		i += 2
		j++
	}
	d.freq[arcvT] = 0xffff // sentinel: stops the update() swap scan at the root
	d.prnt[arcvR] = 0
	return d
}

func (d *arcvDecoder) getBit() int {
	if d.bitcnt == 0 {
		if d.pos < len(d.data) {
			d.bitbuf = d.data[d.pos]
			d.pos++
		} else {
			d.bitbuf = 0
		}
		d.bitcnt = 8
	}
	d.bitcnt--
	return int(d.bitbuf>>d.bitcnt) & 1
}

func (d *arcvDecoder) getByte() int {
	v := 0
	for i := 0; i < 8; i++ {
		v = v<<1 | d.getBit()
	}
	return v
}

// reconst halves all frequencies and rebuilds the tree (LZHUF standard).
func (d *arcvDecoder) reconst() {
	j := 0
	for i := 0; i < arcvT; i++ {
		if d.son[i] >= arcvT {
			d.freq[j] = (d.freq[i] + 1) / 2
			d.son[j] = d.son[i]
			j++
		}
	}
	i, j := 0, arcvNChar
	for j < arcvT {
		f := d.freq[i] + d.freq[i+1]
		k := j - 1
		for f < d.freq[k] {
			k--
		}
		k++
		copy(d.freq[k+1:j+1], d.freq[k:j])
		d.freq[k] = f
		copy(d.son[k+1:j+1], d.son[k:j])
		d.son[k] = int16(i)
		i += 2
		j++
	}
	for i := 0; i < arcvT; i++ {
		k := int(d.son[i])
		d.prnt[k] = int16(i)
		if k < arcvT {
			d.prnt[k+1] = int16(i)
		}
	}
}

// update increments a symbol's frequency and rebalances (LZHUF standard).
func (d *arcvDecoder) update(c int) {
	if d.freq[arcvR] == arcvMaxFreq {
		d.reconst()
	}
	c = int(d.prnt[c+arcvT])
	for {
		d.freq[c]++
		k := d.freq[c]
		l := c + 1
		if k > d.freq[l] {
			for k > d.freq[l+1] {
				l++
			}
			d.freq[c] = d.freq[l]
			d.freq[l] = k
			i := int(d.son[c])
			d.prnt[i] = int16(l)
			if i < arcvT {
				d.prnt[i+1] = int16(l)
			}
			j := int(d.son[l])
			d.son[l] = int16(i)
			d.prnt[j] = int16(c)
			if j < arcvT {
				d.prnt[j+1] = int16(c)
			}
			d.son[c] = int16(j)
			c = l
		}
		c = int(d.prnt[c])
		if c == 0 {
			break
		}
	}
}

func (d *arcvDecoder) decodeChar() int {
	c := int(d.son[arcvR])
	for c < arcvT {
		c = int(d.son[c+d.getBit()])
	}
	c -= arcvT
	d.update(c)
	return c
}

func (d *arcvDecoder) decodePosition() int {
	i := d.getByte()
	c := int(arcvDCode[i]) << 6
	for j := int(arcvDLen[i]) - 2; j > 0; j-- {
		i = i<<1 + d.getBit()
	}
	return c | (i & 0x3f)
}

// Decompress decodes the archive's member file, verifying size and CRC.
func (f *ARCVFile) Decompress() ([]byte, error) {
	if crc := crc32.ChecksumIEEE(f.stream) ^ 0xFFFFFFFF; crc != f.CRC {
		return nil, fmt.Errorf("ARCV data corrupt: CRC %08x, expected %08x", crc, f.CRC)
	}

	d := newARCVDecoder(f.stream)
	var buf [arcvN]byte
	for i := range buf {
		buf[i] = 0x20
	}
	r := arcvN - arcvT // 3523: EDI's quirk (LZHUF uses N-F here)
	out := make([]byte, 0, f.OrigSize)

	for {
		c := d.decodeChar()
		switch {
		case c == 0x100: // end of stream
			if len(out) != int(f.OrigSize) {
				return nil, fmt.Errorf("ARCV decompressed to %d bytes, expected %d", len(out), f.OrigSize)
			}
			return out, nil
		case c < 0x100: // literal
			out = append(out, byte(c))
			buf[r] = byte(c)
			r = (r + 1) & (arcvN - 1)
		default: // match: length 3..32
			pos := d.decodePosition()
			src := (r - pos - 1) & (arcvN - 1)
			length := c - 0x100 + 2
			for k := 0; k < length; k++ {
				ch := buf[(src+k)&(arcvN-1)]
				out = append(out, ch)
				buf[r] = ch
				r = (r + 1) & (arcvN - 1)
			}
		}
		if len(out) > int(f.OrigSize) {
			return nil, fmt.Errorf("ARCV decompression overran expected size %d", f.OrigSize)
		}
	}
}
