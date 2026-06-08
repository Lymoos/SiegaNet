//go:build windows

// Windows Configurator. Routes, inner IP and (later steps) DNS + kill-switch are
// installed through the IP Helper API via thin bindings over iphlpapi.dll (no
// netsh: no localized-output parsing, deterministic error codes, and everything
// is bound to the tunnel's interface LUID rather than a racy interface index).
//
// NOTE (Windows debt): the MIB struct layouts below mirror the Win32 ABI from
// MSDN. They cross-compile here, but their on-the-wire correctness and the real
// route interception are only validated on a live Windows host.
package clientnet

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/tun"

	"github.com/lymoos/sieganet/internal/clientcore"
	"github.com/lymoos/sieganet/internal/tundev"
)

// ---- iphlpapi bindings not provided by x/sys/windows ----

var (
	modiphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")

	procInitializeIpForwardEntry     = modiphlpapi.NewProc("InitializeIpForwardEntry")
	procCreateIpForwardEntry2        = modiphlpapi.NewProc("CreateIpForwardEntry2")
	procDeleteIpForwardEntry2        = modiphlpapi.NewProc("DeleteIpForwardEntry2")
	procGetBestRoute2                = modiphlpapi.NewProc("GetBestRoute2")
	procInitializeUnicastIpAddrEntry = modiphlpapi.NewProc("InitializeUnicastIpAddressEntry")
	procCreateUnicastIpAddressEntry  = modiphlpapi.NewProc("CreateUnicastIpAddressEntry")
	procDeleteUnicastIpAddressEntry  = modiphlpapi.NewProc("DeleteUnicastIpAddressEntry")
	procSetIpInterfaceEntry          = modiphlpapi.NewProc("SetIpInterfaceEntry")
)

// ipAddressPrefix is IP_ADDRESS_PREFIX: a SOCKADDR_INET plus a prefix length,
// padded to a 4-byte boundary (32 bytes total).
type ipAddressPrefix struct {
	Prefix       windows.RawSockaddrInet6
	PrefixLength uint8
	_            [3]byte
}

// mibIpforwardRow2 mirrors MIB_IPFORWARD_ROW2.
type mibIpforwardRow2 struct {
	InterfaceLuid        uint64
	InterfaceIndex       uint32
	DestinationPrefix    ipAddressPrefix
	NextHop              windows.RawSockaddrInet6
	SitePrefixLength     uint8
	ValidLifetime        uint32
	PreferredLifetime    uint32
	Metric               uint32
	Protocol             uint32
	Loopback             uint8
	AutoconfigureAddress uint8
	Publish              uint8
	Immortal             uint8
	Age                  uint32
	Origin               uint32
}

// setInetAddr writes ip into a SOCKADDR_INET (RawSockaddrInet6 holds the union).
// IPv4: family at [0:2], IN_ADDR at [4:8]; IPv6: family at [0:2], addr at [8:24].
func setInetAddr(sa *windows.RawSockaddrInet6, ip netip.Addr) {
	raw := (*[unsafe.Sizeof(*sa)]byte)(unsafe.Pointer(sa))
	*raw = [unsafe.Sizeof(*sa)]byte{}
	if ip.Is4() {
		binary.LittleEndian.PutUint16(raw[0:], uint16(windows.AF_INET))
		a := ip.As4()
		copy(raw[4:8], a[:])
		return
	}
	binary.LittleEndian.PutUint16(raw[0:], uint16(windows.AF_INET6))
	a := ip.As16()
	copy(raw[8:24], a[:])
}

func netioErr(r1 uintptr) error {
	if r1 == 0 {
		return nil
	}
	return windows.Errno(r1)
}

// ---- Configurator ----

// Windows configures the tunnel via the IP Helper API.
type Windows struct {
	luid      uint64
	addrRow   *windows.MibUnicastIpAddressRow // inner IP, for cleanup
	epRoute   *mibIpforwardRow2               // endpoint /32 on the physical iface, for cleanup
	tunRoutes []*mibIpforwardRow2             // full-tunnel routes (auto-removed with the adapter, deleted too for tidiness)
}

// New returns a Windows Configurator.
func New() *Windows { return &Windows{} }

var _ clientcore.Configurator = (*Windows)(nil)

// Sweep removes leftovers from a crashed run (DNS/NRPT and the kill-switch are
// added in later steps; a stale endpoint /32 on the physical interface is benign
// — it just routes endpoint traffic normally — and the dead adapter's routes are
// removed by Windows when the adapter is reaped).
func (w *Windows) Sweep() {}

// EngageKillSwitch is added in step 5.
func (w *Windows) EngageKillSwitch(netip.Addr, string) error { return nil }

// ConfigureTUN assigns the inner IP and MTU, then installs routes IN ORDER: the
// endpoint /32 (off-tunnel, via the physical gateway) FIRST, then the
// full-tunnel halves — so a packet to the server is never captured by the tunnel.
func (w *Windows) ConfigureTUN(dev tun.Device, p clientcore.TUNParams) error {
	luid, ok := tundev.DeviceLUID(dev)
	if !ok {
		return fmt.Errorf("clientnet: tunnel device has no LUID")
	}
	w.luid = luid

	if err := w.setInnerIP(luid, p.InnerIP); err != nil {
		return fmt.Errorf("set inner ip: %w", err)
	}
	if err := w.setMTU(luid, p.InnerMTU); err != nil {
		return fmt.Errorf("set mtu: %w", err)
	}

	for _, op := range PlanRoutes(p.EndpointAddr, p.FullTunnel) {
		if op.ViaTunnel {
			if err := w.addTunnelRoute(luid, op.Dest); err != nil {
				return fmt.Errorf("route %s: %w", op.Dest, err)
			}
		} else {
			// Endpoint exclusion: route via the physical next-hop, NOT the tunnel.
			if err := w.addEndpointRoute(op.Dest.Addr()); err != nil {
				return fmt.Errorf("endpoint route %s: %w", op.Dest, err)
			}
		}
	}
	return nil
}

func (w *Windows) setInnerIP(luid uint64, ip netip.Addr) error {
	row := &windows.MibUnicastIpAddressRow{}
	procInitializeUnicastIpAddrEntry.Call(uintptr(unsafe.Pointer(row)))
	row.InterfaceLuid = luid
	setInetAddr(&row.Address, ip)
	row.OnLinkPrefixLength = uint8(ip.BitLen()) // /32 (or /128): host route on-link
	r1, _, _ := procCreateUnicastIpAddressEntry.Call(uintptr(unsafe.Pointer(row)))
	if err := netioErr(r1); err != nil {
		return err
	}
	w.addrRow = row
	return nil
}

func (w *Windows) setMTU(luid uint64, mtu int) error {
	row := &windows.MibIpInterfaceRow{Family: windows.AF_INET, InterfaceLuid: luid}
	if err := windows.GetIpInterfaceEntry(row); err != nil {
		return err
	}
	row.NlMtu = uint32(mtu)
	// SitePrefixLength must be reset before Set per MSDN to avoid EINVAL.
	row.SitePrefixLength = 0
	r1, _, _ := procSetIpInterfaceEntry.Call(uintptr(unsafe.Pointer(row)))
	return netioErr(r1)
}

func (w *Windows) addTunnelRoute(luid uint64, dest netip.Prefix) error {
	row := w.newRoute()
	row.InterfaceLuid = luid
	setInetAddr(&row.DestinationPrefix.Prefix, dest.Addr())
	row.DestinationPrefix.PrefixLength = uint8(dest.Bits())
	// NextHop left as the unspecified address => on-link via the tunnel.
	setInetAddr(&row.NextHop, unspecifiedLike(dest.Addr()))
	if err := w.createRoute(row); err != nil {
		return err
	}
	w.tunRoutes = append(w.tunRoutes, row)
	return nil
}

func (w *Windows) addEndpointRoute(endpoint netip.Addr) error {
	luid, nextHop, err := bestPhysicalRoute(endpoint)
	if err != nil {
		return err
	}
	row := w.newRoute()
	row.InterfaceLuid = luid
	setInetAddr(&row.DestinationPrefix.Prefix, endpoint)
	row.DestinationPrefix.PrefixLength = uint8(endpoint.BitLen()) // /32 or /128
	setInetAddr(&row.NextHop, nextHop)
	if err := w.createRoute(row); err != nil {
		return err
	}
	w.epRoute = row
	return nil
}

func (w *Windows) newRoute() *mibIpforwardRow2 {
	row := &mibIpforwardRow2{}
	procInitializeIpForwardEntry.Call(uintptr(unsafe.Pointer(row)))
	row.Metric = 0
	return row
}

func (w *Windows) createRoute(row *mibIpforwardRow2) error {
	r1, _, _ := procCreateIpForwardEntry2.Call(uintptr(unsafe.Pointer(row)))
	return netioErr(r1)
}

// bestPhysicalRoute asks the stack for the current best route to dst and returns
// the physical interface LUID and the next-hop (gateway) to reach it — so the
// endpoint /32 is pinned off the tunnel.
func bestPhysicalRoute(dst netip.Addr) (luid uint64, nextHop netip.Addr, err error) {
	var destSA windows.RawSockaddrInet6
	setInetAddr(&destSA, dst)
	var row mibIpforwardRow2
	var bestSrc windows.RawSockaddrInet6
	r1, _, _ := procGetBestRoute2.Call(
		0, // InterfaceLuid (optional)
		0, // InterfaceIndex (optional)
		0, // SourceAddress (optional)
		uintptr(unsafe.Pointer(&destSA)),
		0, // AddressSortOptions
		uintptr(unsafe.Pointer(&row)),
		uintptr(unsafe.Pointer(&bestSrc)),
	)
	if e := netioErr(r1); e != nil {
		return 0, netip.Addr{}, e
	}
	return row.InterfaceLuid, addrFromSockaddr(&row.NextHop), nil
}

// addrFromSockaddr reads an IPv4/IPv6 address back out of a SOCKADDR_INET.
func addrFromSockaddr(sa *windows.RawSockaddrInet6) netip.Addr {
	raw := (*[unsafe.Sizeof(*sa)]byte)(unsafe.Pointer(sa))
	switch binary.LittleEndian.Uint16(raw[0:]) {
	case uint16(windows.AF_INET):
		return netip.AddrFrom4([4]byte{raw[4], raw[5], raw[6], raw[7]})
	case uint16(windows.AF_INET6):
		var a [16]byte
		copy(a[:], raw[8:24])
		return netip.AddrFrom16(a)
	}
	return netip.Addr{}
}

func unspecifiedLike(a netip.Addr) netip.Addr {
	if a.Is4() {
		return netip.IPv4Unspecified()
	}
	return netip.IPv6Unspecified()
}

// Cleanup deletes the inner IP and the endpoint /32 (the only objects not reaped
// automatically when the wintun adapter is destroyed). DNS/NRPT and the
// kill-switch teardown are added in later steps.
func (w *Windows) Cleanup() {
	if w.epRoute != nil {
		procDeleteIpForwardEntry2.Call(uintptr(unsafe.Pointer(w.epRoute)))
		w.epRoute = nil
	}
	for _, r := range w.tunRoutes {
		procDeleteIpForwardEntry2.Call(uintptr(unsafe.Pointer(r)))
	}
	w.tunRoutes = nil
	if w.addrRow != nil {
		procDeleteUnicastIpAddressEntry.Call(uintptr(unsafe.Pointer(w.addrRow)))
		w.addrRow = nil
	}
}
