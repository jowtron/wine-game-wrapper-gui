package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
)

// rtGroupIcon is the PE resource type id for icon groups (RT_GROUP_ICON).
// hiddenIconType is an unused type id we rename it to. Renaming the type
// makes icon lookups find nothing: winemac.drv then leaves the Dock icon
// alone (it reads the first RT_GROUP_ICON of the process's main exe via
// EnumResourceNames), and the .app bundle's icon shows instead. The id
// stays sorted after the standard types (max 24), keeping the resource
// directory valid, and the edit is a reversible in-place 4-byte write.
const (
	rtGroupIcon    = 14
	hiddenIconType = 0x0FFF
)

// hidePEGroupIcon renames the RT_GROUP_ICON resource type in a Win32 PE
// exe so the game's embedded icon can't override the bundle icon in the
// Dock. Returns nil if the exe has no icon resources (nothing to hide).
func hidePEGroupIcon(exePath string) error {
	data, err := os.ReadFile(exePath)
	if err != nil {
		return err
	}
	off, err := findResourceTypeEntry(data, rtGroupIcon)
	if err != nil || off == 0 {
		return err
	}
	binary.LittleEndian.PutUint32(data[off:], hiddenIconType)
	return os.WriteFile(exePath, data, 0644)
}

// findResourceTypeEntry returns the file offset of the root resource
// directory entry whose type id is typeID, or 0 if the exe has no such
// resource type. Errors only on malformed/non-PE files.
func findResourceTypeEntry(data []byte, typeID uint32) (int, error) {
	le := binary.LittleEndian
	if len(data) < 0x40 || !bytes.Equal(data[:2], []byte("MZ")) {
		return 0, fmt.Errorf("not an MZ executable")
	}
	peOff := int(le.Uint32(data[0x3c:]))
	if peOff+24 > len(data) || !bytes.Equal(data[peOff:peOff+4], []byte("PE\x00\x00")) {
		return 0, fmt.Errorf("not a PE executable")
	}
	coff := peOff + 4
	numSections := int(le.Uint16(data[coff+2:]))
	optSize := int(le.Uint16(data[coff+16:]))
	secTable := coff + 20 + optSize

	rsrcPtr := -1
	for i := 0; i < numSections; i++ {
		off := secTable + i*40
		if off+40 > len(data) {
			return 0, fmt.Errorf("truncated section table")
		}
		if bytes.Equal(bytes.TrimRight(data[off:off+8], "\x00"), []byte(".rsrc")) {
			rsrcPtr = int(le.Uint32(data[off+20:]))
			break
		}
	}
	if rsrcPtr < 0 {
		return 0, nil // no resources at all
	}
	if rsrcPtr+16 > len(data) {
		return 0, fmt.Errorf("truncated .rsrc section")
	}
	numEntries := int(le.Uint16(data[rsrcPtr+12:])) + int(le.Uint16(data[rsrcPtr+14:]))
	for i := 0; i < numEntries; i++ {
		entryOff := rsrcPtr + 16 + i*8
		if entryOff+8 > len(data) {
			return 0, fmt.Errorf("truncated resource directory")
		}
		if le.Uint32(data[entryOff:]) == typeID {
			return entryOff, nil
		}
	}
	return 0, nil // no icon groups
}
