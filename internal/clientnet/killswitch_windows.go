//go:build windows

// WFP kill-switch. The filter POLICY lives in killswitch_spec.go (unit-tested);
// this file is the mechanical translation into Windows Filtering Platform calls.
//
// Lifecycle (the no-leak invariants, guaranteed by clientcore's call order):
//   - the engine is opened with a DYNAMIC session and the base block/permit set
//     is installed in ONE transaction BEFORE the first dial, so nothing leaks
//     even during the initial handshake (only loopback, narrow DHCP and the
//     endpoint /32 are reachable off the tunnel);
//   - it stays up for the whole session; a session drop is NOT a teardown, so a
//     disconnect fails closed with no window (clientcore only calls Cleanup at
//     the very end);
//   - a DYNAMIC session means the kernel removes our filters automatically when
//     the process/engine handle dies — a crash fails OPEN (internet restored),
//     the deliberate trade-off, and there is nothing for Sweep to clean up.
//
// The FWPM_* struct layouts are mirrored field-for-field from
// golang.zx2c4.com/wireguard/windows/tunnel/firewall, with compile-time
// size/offset assertions against that package's published constants — so a
// layout drift fails the GOOS=windows build here rather than misbehaving live.
// (The IP Helper / WFP *call* semantics are still validated only on a live host.)
package clientnet

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ---- fwpuclnt bindings ----

var (
	modfwpuclnt = windows.NewLazySystemDLL("fwpuclnt.dll")

	procFwpmEngineOpen0        = modfwpuclnt.NewProc("FwpmEngineOpen0")
	procFwpmEngineClose0       = modfwpuclnt.NewProc("FwpmEngineClose0")
	procFwpmProviderAdd0       = modfwpuclnt.NewProc("FwpmProviderAdd0")
	procFwpmSubLayerAdd0       = modfwpuclnt.NewProc("FwpmSubLayerAdd0")
	procFwpmFilterAdd0         = modfwpuclnt.NewProc("FwpmFilterAdd0")
	procFwpmFilterDeleteById0  = modfwpuclnt.NewProc("FwpmFilterDeleteById0")
	procFwpmTransactionBegin0  = modfwpuclnt.NewProc("FwpmTransactionBegin0")
	procFwpmTransactionCommit0 = modfwpuclnt.NewProc("FwpmTransactionCommit0")
	procFwpmTransactionAbort0  = modfwpuclnt.NewProc("FwpmTransactionAbort0")
)

func wfpErr(r uintptr) error {
	if r == 0 {
		return nil
	}
	return windows.Errno(r)
}

// ---- enums (mirrored from the reference) ----

type wtFwpDataType uint

const (
	cFWP_UINT8  wtFwpDataType = 1
	cFWP_UINT16 wtFwpDataType = 2
	cFWP_UINT32 wtFwpDataType = 3
	cFWP_UINT64 wtFwpDataType = 4
)

type wtFwpMatchType uint32

const (
	cFWP_MATCH_EQUAL         wtFwpMatchType = 0
	cFWP_MATCH_FLAGS_ALL_SET wtFwpMatchType = 6
)

type wtFwpActionType uint32

const (
	cFWP_ACTION_FLAG_TERMINATING wtFwpActionType = 0x00001000
	cFWP_ACTION_BLOCK            wtFwpActionType = 0x00000001 | cFWP_ACTION_FLAG_TERMINATING
	cFWP_ACTION_PERMIT           wtFwpActionType = 0x00000002 | cFWP_ACTION_FLAG_TERMINATING
)

const (
	cFWP_CONDITION_FLAG_IS_LOOPBACK uint32 = 0x00000001
	cFWPM_SESSION_FLAG_DYNAMIC      uint32 = 0x00000001
	cRPC_C_AUTHN_WINNT              uint32 = 10
)

// ---- structs (mirrored field-for-field from the reference) ----

type wtFwpByteBlob struct {
	size uint32
	data *uint8
}

type wtFwpmDisplayData0 struct {
	name        *uint16
	description *uint16
}

type wtFwpValue0 struct {
	_type wtFwpDataType
	value uintptr
}

type wtFwpConditionValue0 struct {
	_type wtFwpDataType
	value uintptr
}

type wtFwpmAction0 struct {
	_type      wtFwpActionType
	filterType windows.GUID
}

type wtFwpmFilterCondition0 struct {
	fieldKey       windows.GUID
	matchType      wtFwpMatchType
	conditionValue wtFwpConditionValue0
}

type wtFwpmFilter0 struct {
	filterKey           windows.GUID
	displayData         wtFwpmDisplayData0
	flags               uint32
	providerKey         *windows.GUID
	providerData        wtFwpByteBlob
	layerKey            windows.GUID
	subLayerKey         windows.GUID
	weight              wtFwpValue0
	numFilterConditions uint32
	filterCondition     *wtFwpmFilterCondition0
	action              wtFwpmAction0
	offset1             [4]byte // layout-correction field (per the reference)
	providerContextKey  windows.GUID
	reserved            *windows.GUID
	filterID            uint64
	effectiveWeight     wtFwpValue0
}

type wtFwpmSession0 struct {
	sessionKey           windows.GUID
	displayData          wtFwpmDisplayData0
	flags                uint32
	txnWaitTimeoutInMSec uint32
	processId            uint32
	sid                  *windows.SID
	username             *uint16
	kernelMode           uint8
}

type wtFwpmSublayer0 struct {
	subLayerKey  windows.GUID
	displayData  wtFwpmDisplayData0
	flags        uint32
	providerKey  *windows.GUID
	providerData wtFwpByteBlob
	weight       uint16
}

type wtFwpProvider0 struct {
	providerKey  windows.GUID
	displayData  wtFwpmDisplayData0
	flags        uint32
	providerData wtFwpByteBlob
	serviceName  *uint16
}

// Compile-time ABI assertions against the reference's published sizes/offsets
// (types_windows_64.go). Any drift underflows an unsigned const => build fails.
const (
	_ = uint(unsafe.Sizeof(wtFwpByteBlob{})) - 16
	_ = 16 - uint(unsafe.Sizeof(wtFwpByteBlob{}))
	_ = uint(unsafe.Sizeof(wtFwpValue0{})) - 16
	_ = 16 - uint(unsafe.Sizeof(wtFwpValue0{}))
	_ = uint(unsafe.Offsetof(wtFwpValue0{}.value)) - 8
	_ = 8 - uint(unsafe.Offsetof(wtFwpValue0{}.value))
	_ = uint(unsafe.Sizeof(wtFwpConditionValue0{})) - 16
	_ = 16 - uint(unsafe.Sizeof(wtFwpConditionValue0{}))
	_ = uint(unsafe.Sizeof(wtFwpmFilterCondition0{})) - 40
	_ = 40 - uint(unsafe.Sizeof(wtFwpmFilterCondition0{}))
	_ = uint(unsafe.Offsetof(wtFwpmFilterCondition0{}.conditionValue)) - 24
	_ = 24 - uint(unsafe.Offsetof(wtFwpmFilterCondition0{}.conditionValue))
	_ = uint(unsafe.Sizeof(wtFwpmFilter0{})) - 200
	_ = 200 - uint(unsafe.Sizeof(wtFwpmFilter0{}))
	_ = uint(unsafe.Offsetof(wtFwpmFilter0{}.weight)) - 96
	_ = 96 - uint(unsafe.Offsetof(wtFwpmFilter0{}.weight))
	_ = uint(unsafe.Offsetof(wtFwpmFilter0{}.filterCondition)) - 120
	_ = 120 - uint(unsafe.Offsetof(wtFwpmFilter0{}.filterCondition))
	_ = uint(unsafe.Offsetof(wtFwpmFilter0{}.action)) - 128
	_ = 128 - uint(unsafe.Offsetof(wtFwpmFilter0{}.action))
	_ = uint(unsafe.Offsetof(wtFwpmFilter0{}.providerContextKey)) - 152
	_ = 152 - uint(unsafe.Offsetof(wtFwpmFilter0{}.providerContextKey))
	_ = uint(unsafe.Sizeof(wtFwpmSession0{})) - 72
	_ = 72 - uint(unsafe.Sizeof(wtFwpmSession0{}))
	_ = uint(unsafe.Sizeof(wtFwpmSublayer0{})) - 72
	_ = 72 - uint(unsafe.Sizeof(wtFwpmSublayer0{}))
	_ = uint(unsafe.Sizeof(wtFwpProvider0{})) - 64
	_ = 64 - uint(unsafe.Sizeof(wtFwpProvider0{}))
)

// ---- condition/layer GUIDs (mirrored from the reference) ----

var (
	cFWPM_CONDITION_IP_LOCAL_INTERFACE = windows.GUID{Data1: 0x4cd62a49, Data2: 0x59c3, Data3: 0x4969, Data4: [8]byte{0xb7, 0xf3, 0xbd, 0xa5, 0xd3, 0x28, 0x90, 0xa4}}
	cFWPM_CONDITION_IP_REMOTE_ADDRESS  = windows.GUID{Data1: 0xb235ae9a, Data2: 0x1d64, Data3: 0x49b8, Data4: [8]byte{0xa4, 0x4c, 0x5f, 0xf3, 0xd9, 0x09, 0x50, 0x45}}
	cFWPM_CONDITION_IP_PROTOCOL        = windows.GUID{Data1: 0x3971ef2b, Data2: 0x623e, Data3: 0x4f9a, Data4: [8]byte{0x8c, 0xb1, 0x6e, 0x79, 0xb8, 0x06, 0xb9, 0xa7}}
	cFWPM_CONDITION_IP_LOCAL_PORT      = windows.GUID{Data1: 0x0c1ba1af, Data2: 0x5765, Data3: 0x453f, Data4: [8]byte{0xaf, 0x22, 0xa8, 0xf7, 0x91, 0xac, 0x77, 0x5b}}
	cFWPM_CONDITION_IP_REMOTE_PORT     = windows.GUID{Data1: 0xc35a604d, Data2: 0xd22b, Data3: 0x4e1a, Data4: [8]byte{0x91, 0xb4, 0x68, 0xf6, 0x74, 0xee, 0x67, 0x4b}}
	cFWPM_CONDITION_FLAGS              = windows.GUID{Data1: 0x632ce23b, Data2: 0x5167, Data3: 0x435c, Data4: [8]byte{0x86, 0xd7, 0xe9, 0x03, 0x68, 0x4a, 0xa8, 0x0c}}

	cFWPM_LAYER_ALE_AUTH_CONNECT_V4     = windows.GUID{Data1: 0xc38d57d1, Data2: 0x05a7, Data3: 0x4c33, Data4: [8]byte{0x90, 0x4f, 0x7f, 0xbc, 0xee, 0xe6, 0x0e, 0x82}}
	cFWPM_LAYER_ALE_AUTH_RECV_ACCEPT_V4 = windows.GUID{Data1: 0xe1cd9fe7, Data2: 0xf4b5, Data3: 0x4273, Data4: [8]byte{0x96, 0xc0, 0x59, 0x2e, 0x48, 0x7b, 0x86, 0x50}}
	cFWPM_LAYER_ALE_AUTH_CONNECT_V6     = windows.GUID{Data1: 0x4a72393b, Data2: 0x319f, Data3: 0x44bc, Data4: [8]byte{0x84, 0xc3, 0xba, 0x54, 0xdc, 0xb3, 0xb6, 0xb4}}
	cFWPM_LAYER_ALE_AUTH_RECV_ACCEPT_V6 = windows.GUID{Data1: 0xa3b42c97, Data2: 0x9f04, Data3: 0x4672, Data4: [8]byte{0xb8, 0x7e, 0xce, 0xe9, 0xc4, 0x83, 0x25, 0x7f}}

	// SiegaNet's own provider + sublayer (fixed, recognisable).
	siegaProviderGUID = windows.GUID{Data1: 0x51e6a4e0, Data2: 0x9b27, Data3: 0x4f3a, Data4: [8]byte{0xa1, 0x70, 0x53, 0x69, 0x65, 0x67, 0x61, 0x50}}
	siegaSublayerGUID = windows.GUID{Data1: 0x51e6a4e1, Data2: 0x9b27, Data3: 0x4f3a, Data4: [8]byte{0xa1, 0x70, 0x53, 0x69, 0x65, 0x67, 0x61, 0x53}}
)

func layerGUID(l fwLayer) windows.GUID {
	switch l {
	case layerConnectV4:
		return cFWPM_LAYER_ALE_AUTH_CONNECT_V4
	case layerRecvV4:
		return cFWPM_LAYER_ALE_AUTH_RECV_ACCEPT_V4
	case layerConnectV6:
		return cFWPM_LAYER_ALE_AUTH_CONNECT_V6
	default:
		return cFWPM_LAYER_ALE_AUTH_RECV_ACCEPT_V6
	}
}

func conditionFieldGUID(f fwField) windows.GUID {
	switch f {
	case fieldLoopback:
		return cFWPM_CONDITION_FLAGS
	case fieldLocalInterface:
		return cFWPM_CONDITION_IP_LOCAL_INTERFACE
	case fieldRemoteAddr:
		return cFWPM_CONDITION_IP_REMOTE_ADDRESS
	case fieldProtocol:
		return cFWPM_CONDITION_IP_PROTOCOL
	case fieldLocalPort:
		return cFWPM_CONDITION_IP_LOCAL_PORT
	default:
		return cFWPM_CONDITION_IP_REMOTE_PORT
	}
}

func actionType(a fwAction) wtFwpActionType {
	if a == actionPermit {
		return cFWP_ACTION_PERMIT
	}
	return cFWP_ACTION_BLOCK
}

// ---- engine ----

type wfpEngine struct {
	handle           uintptr
	endpointFilterID uint64
	tunnelDone       bool
}

func (e *wfpEngine) transaction(body func() error) error {
	if r, _, _ := procFwpmTransactionBegin0.Call(e.handle, 0); wfpErr(r) != nil {
		return fmt.Errorf("wfp txn begin: %w", wfpErr(r))
	}
	if err := body(); err != nil {
		procFwpmTransactionAbort0.Call(e.handle)
		return err
	}
	if r, _, _ := procFwpmTransactionCommit0.Call(e.handle); wfpErr(r) != nil {
		return fmt.Errorf("wfp txn commit: %w", wfpErr(r))
	}
	return nil
}

func (e *wfpEngine) addProvider() error {
	name, _ := windows.UTF16PtrFromString("SiegaNet")
	p := wtFwpProvider0{providerKey: siegaProviderGUID, displayData: wtFwpmDisplayData0{name: name}}
	r, _, _ := procFwpmProviderAdd0.Call(e.handle, uintptr(unsafe.Pointer(&p)), 0)
	runtime.KeepAlive(name)
	if err := wfpErr(r); err != nil && err != windows.Errno(0x80320009) /* already exists */ {
		return fmt.Errorf("wfp provider add: %w", err)
	}
	return nil
}

func (e *wfpEngine) addSublayer() error {
	name, _ := windows.UTF16PtrFromString("SiegaNet kill-switch")
	s := wtFwpmSublayer0{
		subLayerKey: siegaSublayerGUID,
		displayData: wtFwpmDisplayData0{name: name},
		providerKey: &siegaProviderGUID,
		weight:      0xffff,
	}
	r, _, _ := procFwpmSubLayerAdd0.Call(e.handle, uintptr(unsafe.Pointer(&s)), 0)
	runtime.KeepAlive(name)
	if err := wfpErr(r); err != nil {
		return fmt.Errorf("wfp sublayer add: %w", err)
	}
	return nil
}

// addFilter installs one filter and returns its kernel filter ID.
func (e *wfpEngine) addFilter(f fwFilter) (uint64, error) {
	conds := make([]wtFwpmFilterCondition0, len(f.Conditions))
	luids := make([]uint64, len(f.Conditions))
	for i, c := range f.Conditions {
		cc := &conds[i]
		cc.fieldKey = conditionFieldGUID(c.Field)
		cc.matchType = cFWP_MATCH_EQUAL
		switch c.Field {
		case fieldLoopback:
			cc.matchType = cFWP_MATCH_FLAGS_ALL_SET
			cc.conditionValue._type = cFWP_UINT32
			cc.conditionValue.value = uintptr(cFWP_CONDITION_FLAG_IS_LOOPBACK)
		case fieldLocalInterface:
			luids[i] = c.LUID
			cc.conditionValue._type = cFWP_UINT64
			cc.conditionValue.value = uintptr(unsafe.Pointer(&luids[i]))
		case fieldRemoteAddr:
			// A single /32 endpoint is FWP_UINT32 in host byte order, stored
			// INLINE — NOT FWP_V4_ADDR_MASK (that is an addr+mask range passed by
			// pointer; using it for an exact address is what the live WFP engine
			// rejected with "FWP_VALUE ... is of the wrong type"). This mirrors
			// wireguard-windows/tunnel/firewall's permit-by-address rule.
			a := c.Addr.As4()
			cc.conditionValue._type = cFWP_UINT32
			cc.conditionValue.value = uintptr(binary.BigEndian.Uint32(a[:]))
		case fieldProtocol:
			cc.conditionValue._type = cFWP_UINT8
			cc.conditionValue.value = uintptr(c.Proto)
		case fieldLocalPort, fieldRemotePort:
			cc.conditionValue._type = cFWP_UINT16
			cc.conditionValue.value = uintptr(c.Port)
		}
	}

	name, _ := windows.UTF16PtrFromString("SiegaNet " + f.Name)
	filter := wtFwpmFilter0{
		displayData: wtFwpmDisplayData0{name: name},
		providerKey: &siegaProviderGUID,
		layerKey:    layerGUID(f.Layer),
		subLayerKey: siegaSublayerGUID,
		weight:      wtFwpValue0{_type: cFWP_UINT8, value: uintptr(f.Weight)},
		action:      wtFwpmAction0{_type: actionType(f.Action)},
	}
	if len(conds) > 0 {
		filter.numFilterConditions = uint32(len(conds))
		filter.filterCondition = &conds[0]
	}

	var id uint64
	r, _, _ := procFwpmFilterAdd0.Call(e.handle, uintptr(unsafe.Pointer(&filter)), 0, uintptr(unsafe.Pointer(&id)))
	runtime.KeepAlive(conds)
	runtime.KeepAlive(luids)
	runtime.KeepAlive(name)
	if err := wfpErr(r); err != nil {
		return 0, fmt.Errorf("wfp filter add %q: %w", f.Name, err)
	}
	return id, nil
}

// engageWFP opens a dynamic engine and installs the base filter set (block-all +
// loopback + DHCP + endpoint; the tunnel permit is added later, once the adapter
// exists, via permitTunnel) in one transaction.
func engageWFP(endpoint netip.Addr) (*wfpEngine, error) {
	session := wtFwpmSession0{flags: cFWPM_SESSION_FLAG_DYNAMIC}
	var handle uintptr
	r, _, _ := procFwpmEngineOpen0.Call(0, uintptr(cRPC_C_AUTHN_WINNT), 0,
		uintptr(unsafe.Pointer(&session)), uintptr(unsafe.Pointer(&handle)))
	if err := wfpErr(r); err != nil {
		return nil, fmt.Errorf("wfp engine open: %w", err)
	}
	e := &wfpEngine{handle: handle}
	err := e.transaction(func() error {
		if err := e.addProvider(); err != nil {
			return err
		}
		if err := e.addSublayer(); err != nil {
			return err
		}
		for _, f := range killSwitchSpec(endpoint, 0) { // 0 => no tunnel permit yet
			id, err := e.addFilter(f)
			if err != nil {
				return err
			}
			if f.Name == "permit-endpoint" {
				e.endpointFilterID = id
			}
		}
		return nil
	})
	if err != nil {
		e.close()
		return nil, err
	}
	return e, nil
}

// permitTunnel adds the tunnel-interface permits once the adapter LUID is known.
func (e *wfpEngine) permitTunnel(luid uint64) error {
	if e.tunnelDone {
		return nil
	}
	err := e.transaction(func() error {
		for _, f := range tunnelPermits(luid) {
			if _, err := e.addFilter(f); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		e.tunnelDone = true
	}
	return err
}

// updateEndpoint swaps the endpoint /32 permit atomically (Phase 4 failover).
func (e *wfpEngine) updateEndpoint(endpoint netip.Addr) error {
	f, ok := endpointPermit(endpoint)
	if !ok {
		return nil
	}
	return e.transaction(func() error {
		if e.endpointFilterID != 0 {
			r, _, _ := procFwpmFilterDeleteById0.Call(e.handle, uintptr(e.endpointFilterID))
			if err := wfpErr(r); err != nil {
				return fmt.Errorf("wfp delete endpoint filter: %w", err)
			}
		}
		id, err := e.addFilter(f)
		if err != nil {
			return err
		}
		e.endpointFilterID = id
		return nil
	})
}

// close releases the engine; the DYNAMIC session removes all our filters.
func (e *wfpEngine) close() {
	if e.handle != 0 {
		procFwpmEngineClose0.Call(e.handle)
		e.handle = 0
	}
}
