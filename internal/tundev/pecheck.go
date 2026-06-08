package tundev

import (
	"encoding/binary"
	"fmt"
)

// PE/COFF machine types we care about.
const (
	peMachineI386  uint16 = 0x014c
	peMachineAMD64 uint16 = 0x8664
	peMachineARM64 uint16 = 0xaa64
)

// peMachine extracts the COFF machine type from a PE (DLL/EXE) image. It parses
// only the few bytes it needs, so it is OS-neutral and unit-testable anywhere:
//
//	"MZ" at 0; little-endian PE-header offset at 0x3C; "PE\0\0" signature there;
//	the 2-byte Machine field immediately follows the signature.
func peMachine(data []byte) (uint16, error) {
	if len(data) < 0x40 {
		return 0, fmt.Errorf("not a PE image: too short (%d bytes)", len(data))
	}
	if data[0] != 'M' || data[1] != 'Z' {
		return 0, fmt.Errorf("not a PE image: missing MZ signature")
	}
	peOff := int(binary.LittleEndian.Uint32(data[0x3C:0x40]))
	if peOff < 0 || peOff+6 > len(data) {
		return 0, fmt.Errorf("not a PE image: bad header offset %d", peOff)
	}
	if data[peOff] != 'P' || data[peOff+1] != 'E' || data[peOff+2] != 0 || data[peOff+3] != 0 {
		return 0, fmt.Errorf("not a PE image: missing PE signature")
	}
	return binary.LittleEndian.Uint16(data[peOff+4 : peOff+6]), nil
}

// machineName renders a machine type for error messages.
func machineName(m uint16) string {
	switch m {
	case peMachineAMD64:
		return "amd64"
	case peMachineARM64:
		return "arm64"
	case peMachineI386:
		return "386"
	default:
		return fmt.Sprintf("0x%04x", m)
	}
}

// machineForGOARCH maps a Go architecture to its PE machine type.
func machineForGOARCH(goarch string) (uint16, bool) {
	switch goarch {
	case "amd64":
		return peMachineAMD64, true
	case "arm64":
		return peMachineARM64, true
	case "386":
		return peMachineI386, true
	default:
		return 0, false
	}
}

// checkPEArch verifies a PE image's machine type matches want, returning a clear
// error (never a panic) on mismatch or malformed input.
func checkPEArch(data []byte, want uint16) error {
	got, err := peMachine(data)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("wrong architecture: image is %s, this build needs %s",
			machineName(got), machineName(want))
	}
	return nil
}
