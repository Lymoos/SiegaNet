package tundev

import (
	"encoding/binary"
	"testing"
)

// makePE builds a minimal valid PE image header with the given machine type, so
// the parser can be exercised without a real DLL. peOff is where the PE header
// (signature + COFF machine) lives.
func makePE(machine uint16, peOff int) []byte {
	size := peOff + 6
	if size < 0x40 {
		size = 0x40
	}
	data := make([]byte, size)
	data[0], data[1] = 'M', 'Z'
	binary.LittleEndian.PutUint32(data[0x3C:], uint32(peOff))
	copy(data[peOff:], []byte{'P', 'E', 0, 0})
	binary.LittleEndian.PutUint16(data[peOff+4:], machine)
	return data
}

func TestPEMachineParsesArch(t *testing.T) {
	cases := map[uint16]string{
		peMachineAMD64: "amd64",
		peMachineARM64: "arm64",
		peMachineI386:  "386",
	}
	for machine, name := range cases {
		data := makePE(machine, 0x80)
		got, err := peMachine(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != machine {
			t.Errorf("%s: got %#x want %#x", name, got, machine)
		}
		if machineName(got) != name {
			t.Errorf("machineName(%#x) = %q want %q", got, machineName(got), name)
		}
	}
}

func TestCheckPEArchMatchAndMismatch(t *testing.T) {
	amd64 := makePE(peMachineAMD64, 0x40)
	if err := checkPEArch(amd64, peMachineAMD64); err != nil {
		t.Errorf("matching arch rejected: %v", err)
	}
	// An arm64 DLL on an amd64 build must be a clear error, not a panic.
	if err := checkPEArch(makePE(peMachineARM64, 0x40), peMachineAMD64); err == nil {
		t.Error("arch mismatch accepted")
	}
}

func TestPEMachineRejectsMalformed(t *testing.T) {
	cases := map[string][]byte{
		"too short": {0x4d, 0x5a},
		"no MZ":     make([]byte, 0x80),
		"bad pe offset": func() []byte {
			d := makePE(peMachineAMD64, 0x40)
			binary.LittleEndian.PutUint32(d[0x3C:], 0xffffff)
			return d
		}(),
		"no PE sig": func() []byte { d := makePE(peMachineAMD64, 0x40); d[0x40] = 'X'; return d }(),
	}
	for name, data := range cases {
		if _, err := peMachine(data); err == nil {
			t.Errorf("%s: expected error, got none", name)
		}
	}
}

func TestMachineForGOARCH(t *testing.T) {
	for arch, want := range map[string]uint16{"amd64": peMachineAMD64, "arm64": peMachineARM64, "386": peMachineI386} {
		got, ok := machineForGOARCH(arch)
		if !ok || got != want {
			t.Errorf("machineForGOARCH(%q) = %#x,%v want %#x", arch, got, ok, want)
		}
	}
	if _, ok := machineForGOARCH("riscv64"); ok {
		t.Error("unsupported arch reported as supported")
	}
}
