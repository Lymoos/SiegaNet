//go:build windows

package tundev

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/tun"
)

// adapterGUID is a fixed GUID so the SiegaNet wintun adapter has a stable
// identity across runs; the crash-cleanup sweep uses it to find and remove an
// adapter left behind by a previous, non-graceful exit.
var adapterGUID = windows.GUID{
	Data1: 0x5c1e6a4e,
	Data2: 0x9b27,
	Data3: 0x4f3a,
	Data4: [8]byte{0xa1, 0x70, 0x53, 0x69, 0x65, 0x67, 0x61, 0x4e}, // "...SiegaN"
}

// AdapterGUID exposes the fixed adapter GUID for the Windows network layer.
func AdapterGUID() windows.GUID { return adapterGUID }

// Open creates a wintun TUN device with the given name and MTU.
//
// Before touching the driver it validates that wintun.dll sits next to the
// executable and matches this build's architecture, so a missing or wrong-arch
// DLL yields a clear error instead of a panic from the lazy DLL loader. A
// recover() guards the create call as a final backstop.
func Open(name string, mtu int) (dev tun.Device, err error) {
	if e := validateWintunDLL(); e != nil {
		return nil, e
	}
	defer func() {
		if r := recover(); r != nil {
			dev, err = nil, fmt.Errorf("wintun: creating adapter %q panicked (wintun.dll invalid or driver missing?): %v", name, r)
		}
	}()
	d, e := tun.CreateTUNWithRequestedGUID(name, &adapterGUID, mtu)
	if e != nil {
		return nil, fmt.Errorf("wintun: create adapter %q: %w", name, e)
	}
	return d, nil
}

// DeviceLUID returns the wintun adapter's interface LUID, used by the Windows
// network layer to bind routes/DNS/firewall to the tunnel deterministically.
func DeviceLUID(dev tun.Device) (uint64, bool) {
	type luider interface{ LUID() uint64 }
	if l, ok := dev.(luider); ok {
		return l.LUID(), true
	}
	return 0, false
}

// wintunPath returns the expected wintun.dll path next to the executable.
func wintunPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), "wintun.dll"), nil
}

// validateWintunDLL checks that wintun.dll exists next to the exe and matches
// this build's architecture, returning an actionable error otherwise.
func validateWintunDLL() error {
	want, ok := machineForGOARCH(runtime.GOARCH)
	if !ok {
		return fmt.Errorf("wintun: unsupported architecture %q", runtime.GOARCH)
	}
	path, err := wintunPath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("wintun.dll not found next to the executable (expected %s); "+
			"download the %s wintun.dll from wireguard.com and place it beside the exe", path, machineName(want))
	}
	if err := checkPEArch(data, want); err != nil {
		return fmt.Errorf("wintun.dll at %s: %w", path, err)
	}
	return nil
}
