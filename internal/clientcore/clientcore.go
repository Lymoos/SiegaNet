// Package clientcore is the OS-neutral client supervisor. It owns the connect /
// reconnect lifecycle and drives an OS-specific Configurator (routes, inner IP,
// DNS, kill-switch, cleanup). The data-plane, auth and transport are reused
// unchanged: the supervisor only sees a Session (a tunnel.Datagrammer it can
// close) produced by an injected Dial function, so it carries no QUIC- or
// WebTransport-specific assumptions and is fully testable with a fake dialer.
//
// Reconnect design: the TUN device, its routes/DNS and the kill-switch are set
// up ONCE and stay stable for the whole client lifetime (a stable interface LUID
// is what the Windows kill-switch permit binds to). Only the transport session
// is re-dialed on a drop, hidden from tunnel.Run behind reconnectingSession, so
// the data-plane never restarts and no leak window opens.
//
// Reuse: Options.Run is the single entry point for connect + reconnect + ordered
// teardown. The CLI (cmd/sieganet-client) is a thin wrapper around it; the Phase 4
// Windows service / tray will wrap the same Options.Run with a service-managed
// context and need no rewrite of the connect/cleanup logic.
//
// Endpoint resolution policy (Phase 2): the caller resolves the server domain to
// an IP ONCE, before the kill-switch is engaged, and passes that fixed address
// in. Every reconnect — including after a network change — dials the cached IP;
// the supervisor never performs DNS while the kill-switch (which blocks DNS) is
// active. (Endpoint IP rotation is a Phase 4 concern.)
package clientcore

import (
	"context"
	"math/rand/v2"
	"net/netip"
	"time"

	"golang.zx2c4.com/wireguard/tun"

	"github.com/lymoos/sieganet/internal/tunnel"
)

// Session is a dialed tunnel transport: an unreliable datagram channel that can
// be closed. *webtransport.Session satisfies this once wrapped with Close().
type Session interface {
	tunnel.Datagrammer
	Close() error
}

// TUNParams describes how to configure an established tunnel device.
type TUNParams struct {
	TUNName      string
	InnerIP      netip.Addr
	InnerMTU     int
	DNS          string
	SetDNS       bool
	FullTunnel   bool
	EndpointAddr netip.Addr // cached server IP, for the exclusion route/permit
	EndpointPort string
	EndpointDev  string // physical egress device (Linux); derived on Windows
}

// Configurator is the OS-specific network layer.
type Configurator interface {
	// Sweep removes stale state left by a previously-crashed run. Called once at
	// startup, before anything else.
	Sweep()
	// EngageKillSwitch installs the fail-closed firewall permitting only the
	// tunnel, loopback, DHCP and the server endpoint. Called once at startup
	// BEFORE the first dial; stays active for the whole session. No-op when
	// disabled.
	EngageKillSwitch(endpoint netip.Addr, port string) error
	// ConfigureTUN assigns the inner IP, MTU, routes and DNS for dev.
	ConfigureTUN(dev tun.Device, p TUNParams) error
	// Cleanup restores routes/DNS/firewall. Safe to call repeatedly.
	Cleanup()
}

// Options parameterise a run.
type Options struct {
	Configurator Configurator
	Params       TUNParams
	KillSwitch   bool
	OpenTUN      func(name string, mtu int) (tun.Device, error)
	Dial         func(ctx context.Context) (Session, error)
	// MaxDatagram probes a session's max datagram payload (transport-specific,
	// injected so clientcore stays transport-neutral).
	MaxDatagram func(Session) int
	// TunnelConfig is passed to tunnel.Run (padding, stats, etc.). MaxDatagram
	// and LocalTunIP are filled in by the supervisor.
	TunnelConfig tunnel.Config
	Log          func(format string, args ...any)
}

func (o Options) logf(format string, args ...any) {
	if o.Log != nil {
		o.Log(format, args...)
	}
}

// Run drives the lifecycle until ctx is cancelled.
func (o Options) Run(ctx context.Context) error {
	conf := o.Configurator
	conf.Sweep()
	if o.KillSwitch {
		if err := conf.EngageKillSwitch(o.Params.EndpointAddr, o.Params.EndpointPort); err != nil {
			return err
		}
		o.logf("kill-switch engaged (only %s and the tunnel permitted)", o.Params.EndpointAddr)
	}

	// First connection (retry under the kill-switch until success or ctx done).
	first, err := o.dialLoop(ctx)
	if err != nil {
		conf.Cleanup()
		return err
	}

	maxDg := o.MaxDatagram(first)
	dev, err := o.OpenTUN(o.Params.TUNName, o.Params.InnerMTU)
	if err != nil {
		_ = first.Close()
		conf.Cleanup()
		return err
	}

	p := o.Params
	p.InnerMTU = tunnel.InnerMTU(maxDg, o.TunnelConfig.PadMax, o.Params.InnerMTU)
	if err := conf.ConfigureTUN(dev, p); err != nil {
		_ = first.Close()
		conf.Cleanup()
		_ = dev.Close()
		return err
	}
	o.logf("tunnel up: inner=%s mtu=%d full_tunnel=%t dns=%s", p.InnerIP, p.InnerMTU, p.FullTunnel, p.DNS)

	// Hide session churn from the data-plane: one tunnel.Run over a holder that
	// re-dials underneath it. TUN/routes/DNS/kill-switch stay put.
	h := newReconnectingSession(first, o.Dial, o.logf)
	go h.redialLoop(ctx)

	tc := o.TunnelConfig
	tc.MaxDatagram = maxDg
	tc.LocalTunIP = o.Params.InnerIP.AsSlice()
	runErr := tunnel.Run(ctx, dev, h, tc)

	// Ordered teardown (no leak): stop the pump, then undo the network config
	// (the Configurator removes NRPT, restores DNS, then closes the WFP engine so
	// the block lifts only after the tunnel is down), then close the adapter so
	// its inner IP and routes go with it.
	h.close()
	conf.Cleanup()
	_ = dev.Close()
	return runErr
}

// dialLoop retries Dial with backoff until it succeeds or ctx is cancelled.
func (o Options) dialLoop(ctx context.Context) (Session, error) {
	b := backoff{min: time.Second, max: 30 * time.Second}
	for {
		sess, err := o.Dial(ctx)
		if err == nil {
			return sess, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		o.logf("connect failed (%v); retrying", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(b.next()):
		}
	}
}

// backoff is exponential with ±20% jitter, capped at max.
type backoff struct {
	min, max time.Duration
	cur      time.Duration
}

func (b *backoff) next() time.Duration {
	if b.cur == 0 {
		b.cur = b.min
	} else {
		b.cur *= 2
		if b.cur > b.max {
			b.cur = b.max
		}
	}
	span := int64(b.cur) / 5 // 20%
	jitter := time.Duration(rand.Int64N(2*span+1)) - time.Duration(span)
	return b.cur + jitter
}

func (b *backoff) reset() { b.cur = 0 }
