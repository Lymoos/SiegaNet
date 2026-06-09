//go:build windows

// Windows Configurator. Routes, inner IP and DNS are installed through the IP
// Helper API (no netsh: no localized-output parsing, deterministic error codes,
// everything bound to the tunnel LUID) and the registry (NRPT). DNS-leak
// prevention is layered: an NRPT "." rule forces every name to the tunnel DNS,
// smart multi-homed resolution is disabled, and the step-5 kill-switch blocks any
// stray :53 on the physical link.
//
// The SOCKADDR_INET / IP_ADDRESS_PREFIX / MIB_IPFORWARD_ROW2 layouts below mirror
// golang.zx2c4.com/wireguard/windows/tunnel/winipcfg (the proven definitions
// WireGuard's own Windows client uses): same field order, same 28-byte
// SOCKADDR_INET with the IPv4 address at offset 4 / IPv6 at offset 8, same
// IP_ADDRESS_PREFIX padding. They cross-compile here; real route/DNS behaviour is
// validated on a live Windows host.
package clientnet

import (
	"fmt"
	"net/netip"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
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

// rawSockaddrInet mirrors winipcfg.RawSockaddrInet (SOCKADDR_INET union): a
// 2-byte family followed by 26 bytes of address data (28 bytes total). The IPv4
// address sits at sockaddr offset 4 (data[2:6]); the IPv6 address at offset 8
// (data[6:22]) — exactly as winipcfg's SetAddrPort writes via RawSockaddrInet4/6.
type rawSockaddrInet struct {
	Family uint16
	data   [26]byte
}

func (sa *rawSockaddrInet) setAddr(ip netip.Addr) {
	*sa = rawSockaddrInet{}
	if ip.Is4() {
		sa.Family = uint16(windows.AF_INET)
		a := ip.As4()
		copy(sa.data[2:6], a[:])
		return
	}
	sa.Family = uint16(windows.AF_INET6)
	a := ip.As16()
	copy(sa.data[6:22], a[:])
}

func (sa *rawSockaddrInet) addr() netip.Addr {
	switch sa.Family {
	case uint16(windows.AF_INET):
		return netip.AddrFrom4([4]byte{sa.data[2], sa.data[3], sa.data[4], sa.data[5]})
	case uint16(windows.AF_INET6):
		var a [16]byte
		copy(a[:], sa.data[6:22])
		return netip.AddrFrom16(a)
	}
	return netip.Addr{}
}

// ipAddressPrefix mirrors winipcfg.IPAddressPrefix (IP_ADDRESS_PREFIX).
type ipAddressPrefix struct {
	RawPrefix    rawSockaddrInet
	PrefixLength uint8
	_            [2]byte
}

// mibIpforwardRow2 mirrors winipcfg.MibIPforwardRow2 (MIB_IPFORWARD_ROW2),
// field-for-field.
type mibIpforwardRow2 struct {
	InterfaceLuid        uint64
	InterfaceIndex       uint32
	DestinationPrefix    ipAddressPrefix
	NextHop              rawSockaddrInet
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

// Compile-time ABI assertions: if any size/offset drifts from the Win32 layout
// (matching winipcfg), `GOOS=windows go build` fails HERE rather than misbehaving
// on a live host. A wrong value makes one of the paired unsigned consts underflow.
const (
	_ = uint(unsafe.Sizeof(rawSockaddrInet{})) - 28
	_ = 28 - uint(unsafe.Sizeof(rawSockaddrInet{}))
	_ = uint(unsafe.Sizeof(ipAddressPrefix{})) - 32
	_ = 32 - uint(unsafe.Sizeof(ipAddressPrefix{}))
	_ = uint(unsafe.Sizeof(mibIpforwardRow2{})) - 104
	_ = 104 - uint(unsafe.Sizeof(mibIpforwardRow2{}))
	_ = uint(unsafe.Offsetof(mibIpforwardRow2{}.DestinationPrefix)) - 12
	_ = 12 - uint(unsafe.Offsetof(mibIpforwardRow2{}.DestinationPrefix))
	_ = uint(unsafe.Offsetof(mibIpforwardRow2{}.NextHop)) - 44
	_ = 44 - uint(unsafe.Offsetof(mibIpforwardRow2{}.NextHop))
)

func netioErr(r1 uintptr) error {
	if r1 == 0 {
		return nil
	}
	return windows.Errno(r1)
}

// ---- registry locations ----

const (
	// A fixed NRPT rule key (recognisable for crash sweeps).
	nrptKeyPath = `SYSTEM\CurrentControlSet\Services\Dnscache\Parameters\DnsPolicyConfig\SiegaNet-CatchAll`
	// DNS client policy: disable smart multi-homed name resolution.
	dnsPolicyPath = `SOFTWARE\Policies\Microsoft\Windows NT\DNSClient`
	ifaceDNSBase  = `SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\`
)

// ---- Configurator ----

// Windows configures the tunnel via the IP Helper API + registry.
type Windows struct {
	luid      uint64
	guid      string
	addrRow   *windows.MibUnicastIpAddressRow
	epRoute   *mibIpforwardRow2
	tunRoutes []*mibIpforwardRow2
	dnsOn     bool
}

// New returns a Windows Configurator.
func New() *Windows { return &Windows{guid: guidString(tundev.AdapterGUID())} }

var _ clientcore.Configurator = (*Windows)(nil)

// Sweep removes DNS/NRPT leftovers from a previously-crashed run (these live in
// the registry and are NOT reaped when the adapter dies). Stale routes/IP on the
// dead adapter are removed by Windows when the adapter is reaped; a stale
// endpoint /32 on the physical link is benign.
func (w *Windows) Sweep() {
	removeNRPT()
	removeDNSPolicy()
	clearInterfaceDNS(w.guid)
}

// EngageKillSwitch is added in step 5.
func (w *Windows) EngageKillSwitch(netip.Addr, string) error { return nil }

// ConfigureTUN assigns the inner IP and MTU, installs routes in anti-loop order,
// then points DNS at the tunnel resolver.
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
			if err := w.addEndpointRoute(op.Dest.Addr()); err != nil {
				return fmt.Errorf("endpoint route %s: %w", op.Dest, err)
			}
		}
	}
	if p.SetDNS && p.DNS != "" {
		if err := w.applyDNS(p.DNS); err != nil {
			return fmt.Errorf("dns: %w", err)
		}
	}
	return nil
}

func (w *Windows) setInnerIP(luid uint64, ip netip.Addr) error {
	row := &windows.MibUnicastIpAddressRow{}
	procInitializeUnicastIpAddrEntry.Call(uintptr(unsafe.Pointer(row)))
	row.InterfaceLuid = luid
	(*rawSockaddrInet)(unsafe.Pointer(&row.Address)).setAddr(ip)
	row.OnLinkPrefixLength = uint8(ip.BitLen()) // /32 (or /128) host route
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
	row.SitePrefixLength = 0 // must be cleared before Set (MSDN) to avoid EINVAL
	r1, _, _ := procSetIpInterfaceEntry.Call(uintptr(unsafe.Pointer(row)))
	return netioErr(r1)
}

func (w *Windows) addTunnelRoute(luid uint64, dest netip.Prefix) error {
	row := newRoute()
	row.InterfaceLuid = luid
	row.DestinationPrefix.RawPrefix.setAddr(dest.Addr())
	row.DestinationPrefix.PrefixLength = uint8(dest.Bits())
	row.NextHop.setAddr(unspecifiedLike(dest.Addr())) // on-link via the tunnel
	if err := createRoute(row); err != nil {
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
	row := newRoute()
	row.InterfaceLuid = luid
	row.DestinationPrefix.RawPrefix.setAddr(endpoint)
	row.DestinationPrefix.PrefixLength = uint8(endpoint.BitLen())
	row.NextHop.setAddr(nextHop)
	if err := createRoute(row); err != nil {
		return err
	}
	w.epRoute = row
	return nil
}

func newRoute() *mibIpforwardRow2 {
	row := &mibIpforwardRow2{}
	procInitializeIpForwardEntry.Call(uintptr(unsafe.Pointer(row)))
	return row
}

func createRoute(row *mibIpforwardRow2) error {
	r1, _, _ := procCreateIpForwardEntry2.Call(uintptr(unsafe.Pointer(row)))
	return netioErr(r1)
}

// bestPhysicalRoute returns the physical interface LUID and next-hop (gateway)
// for dst, so the endpoint /32 is pinned off the tunnel.
func bestPhysicalRoute(dst netip.Addr) (luid uint64, nextHop netip.Addr, err error) {
	var destSA rawSockaddrInet
	destSA.setAddr(dst)
	var row mibIpforwardRow2
	var bestSrc rawSockaddrInet
	r1, _, _ := procGetBestRoute2.Call(
		0, 0, 0,
		uintptr(unsafe.Pointer(&destSA)),
		0,
		uintptr(unsafe.Pointer(&row)),
		uintptr(unsafe.Pointer(&bestSrc)),
	)
	if e := netioErr(r1); e != nil {
		return 0, netip.Addr{}, e
	}
	return row.InterfaceLuid, row.NextHop.addr(), nil
}

func unspecifiedLike(a netip.Addr) netip.Addr {
	if a.Is4() {
		return netip.IPv4Unspecified()
	}
	return netip.IPv6Unspecified()
}

// ---- DNS ----

func (w *Windows) applyDNS(dns string) error {
	server, err := netip.ParseAddr(dns)
	if err != nil {
		return err
	}
	if err := setInterfaceDNS(w.guid, dns); err != nil {
		return err
	}
	if err := writeNRPT([]netip.Addr{server}); err != nil {
		return err
	}
	if err := setDNSPolicy(); err != nil {
		return err
	}
	w.dnsOn = true
	return nil
}

func writeNRPT(servers []netip.Addr) error {
	rule := catchAllNRPT(servers)
	key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, nrptKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.SetDWordValue("Version", rule.Version); err != nil {
		return err
	}
	if err := key.SetStringsValue("Name", rule.Names); err != nil { // REG_MULTI_SZ
		return err
	}
	if err := key.SetDWordValue("ConfigOptions", rule.ConfigOptions); err != nil {
		return err
	}
	return key.SetStringValue("GenericDNSServers", rule.serverList())
}

func removeNRPT() { _ = registry.DeleteKey(registry.LOCAL_MACHINE, nrptKeyPath) }

func setDNSPolicy() error {
	key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, dnsPolicyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.SetDWordValue("DisableSmartNameResolution", 1); err != nil {
		return err
	}
	return key.SetDWordValue("DisableParallelAandAAAA", 1)
}

func removeDNSPolicy() {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, dnsPolicyPath, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	_ = key.DeleteValue("DisableSmartNameResolution")
	_ = key.DeleteValue("DisableParallelAandAAAA")
}

func setInterfaceDNS(guid, dns string) error {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, ifaceDNSBase+guid, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue("NameServer", dns)
}

func clearInterfaceDNS(guid string) {
	if guid == "" {
		return
	}
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, ifaceDNSBase+guid, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	_ = key.DeleteValue("NameServer")
}

// guidString formats a windows.GUID as {XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX}.
func guidString(g windows.GUID) string {
	return fmt.Sprintf("{%08X-%04X-%04X-%02X%02X-%02X%02X%02X%02X%02X%02X}",
		g.Data1, g.Data2, g.Data3, g.Data4[0], g.Data4[1],
		g.Data4[2], g.Data4[3], g.Data4[4], g.Data4[5], g.Data4[6], g.Data4[7])
}

// Cleanup tears down DNS, then the routes and inner IP (the latter also go with
// the adapter, but we delete them for tidiness).
func (w *Windows) Cleanup() {
	if w.dnsOn {
		removeNRPT()
		removeDNSPolicy()
		clearInterfaceDNS(w.guid)
		w.dnsOn = false
	}
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
