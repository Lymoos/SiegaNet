package clientnet

import (
	"net/netip"
	"strings"
	"unicode/utf16"
)

// NRPT (Name Resolution Policy Table) is the Windows mechanism that forces DNS
// resolution for a namespace to specific servers, overriding per-interface
// resolvers and "smart multi-homed" parallel queries. A catch-all "." rule sends
// EVERY name to the tunnel DNS, which — together with the kill-switch blocking
// any stray :53 on the physical link — is how the Windows client guarantees DNS
// goes through the tunnel.
//
// This file holds the OS-neutral construction of the rule's registry values so
// the wire format (especially the REG_MULTI_SZ encoding) is unit-testable here;
// the windows file writes them into the registry.

// nrptConfigGenericServers is the ConfigOptions bit selecting GenericDNSServers.
const nrptConfigGenericServers = 0x8

// nrptVersion is the NRPT rule schema version.
const nrptVersion = 2

// nrptRule is the value set for one NRPT registry rule.
type nrptRule struct {
	Names         []string     // namespaces; {"."} = all names
	Servers       []netip.Addr // DNS servers for those names
	ConfigOptions uint32
	Version       uint32
}

// catchAllNRPT builds the "." rule routing all DNS to the given tunnel servers.
func catchAllNRPT(servers []netip.Addr) nrptRule {
	return nrptRule{
		Names:         []string{"."},
		Servers:       servers,
		ConfigOptions: nrptConfigGenericServers,
		Version:       nrptVersion,
	}
}

// serverList renders the GenericDNSServers value (semicolon-separated).
func (r nrptRule) serverList() string {
	parts := make([]string, len(r.Servers))
	for i, s := range r.Servers {
		parts[i] = s.String()
	}
	return strings.Join(parts, ";")
}

// nameMultiSZ renders the Name value as a REG_MULTI_SZ byte blob (UTF-16LE,
// each string null-terminated, the list terminated by a final null). This
// matches what registry.SetStringsValue writes; it is materialised here so the
// exact format is covered by a test.
func (r nrptRule) nameMultiSZ() []byte {
	return multiSZ(r.Names)
}

func multiSZ(strs []string) []byte {
	var b []byte
	put := func(u uint16) { b = append(b, byte(u), byte(u>>8)) }
	for _, s := range strs {
		for _, u := range utf16.Encode([]rune(s)) {
			put(u)
		}
		put(0) // string terminator
	}
	put(0) // list terminator
	return b
}
