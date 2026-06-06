# SiegaNet

Stealth VPN over QUIC, disguised as an HTTP/3 website. For personal use across
**Linux server, Windows client and Android client**. Tunnel data rides on QUIC
DATAGRAMs (RFC 9221); a decoy site and HMAC peer authentication hide the tunnel
from passive and active probing.

> Built strictly phase by phase. Each phase has an acceptance test that must
> pass before the next one starts.

## Principles

1. **No home-grown crypto.** Channel confidentiality/authenticity comes from
   TLS 1.3 in quic-go. Our code is masking, peer auth, data-plane and clients.
   Only the standard library for HMAC.
2. **Tunnel data over QUIC DATAGRAMs, not streams** — reliable-over-reliable
   kills throughput under loss. Streams are for control/auth only.
3. **The server never gives itself away.** Any unauthenticated connection
   (including active probing) gets a real decoy-site response.
4. **No fixed signatures.** Random datagram padding, jittered keepalive, no
   magic plaintext bytes in the outer layer.
5. **No leaks.** Full tunnel + forced DNS through the server, kill-switch on
   clients.

## Status

| Phase | Scope | State |
|-------|-------|-------|
| 0 | PoC tunnel (Linux ↔ Linux, no masking) | ✅ done |
| 1 | Masking, decoy site, WebTransport, HMAC auth, server | ⏳ next |
| 2 | Windows client (wintun, routes, DNS, kill-switch) | — |
| 3 | Android client (gomobile, VpnService) | — |
| 4 | Polish: reconnect, failover, obfusc, metrics, tray/UI | — |

## Layout

```
cmd/
  sieganet-server/   server daemon (Linux)
  sieganet-client/   CLI/daemon client (Windows + Linux for debugging)
  sieganet-ctl/      peer management on the server
internal/
  protocol/   wire format: datagram framing, padding, control messages
  auth/       HMAC peer authentication
  transport/  QUIC/WebTransport wrapper: dial, listen, decoy
  tunnel/     TUN <-> datagram pump, packet routing
  tundev/     per-OS TUN creation/config (linux.go, windows.go, android.go)
  peers/      peer model, IP allocation, store
  obfusc/     optional Salamander-XOR (off by default)
mobile/       thin gomobile-bind layer (primitive types only)
android/      Android Studio project (Kotlin): VpnService + UI
configs/      server.example.toml, client.example.toml
scripts/      test/setup helpers
```

## Phase 0 — how to verify by hand

Phase 0 is a bare QUIC↔TUN bridge: no auth, no masking. It proves the transport
is alive and resilient. Requires Linux, root, iproute2 (`ip`, `tc`), iperf3,
iputils-ping, iptables.

> **Insecure mode is gated behind a build tag.** The self-signed certificate
> and `InsecureSkipVerify` only exist in builds tagged `phase0_insecure`; a
> normal build refuses to produce an insecure TLS config, so the Phase 0
> shortcut cannot leak into Phase 1+. The Phase 0 binaries must therefore be
> built with `-tags phase0_insecure`.

Automated acceptance + resilience test (two network namespaces over a veth
pair: ping, PMTU probe, iperf3, MSS-clamp check, drop-counter attribution, and
a mobile-like delay/loss pass — `tc netem` when available, otherwise the
userspace `siega-impair` UDP relay, which needs no kernel module):

```sh
sudo bash scripts/phase0-netns-test.sh
```

Expected: `ping 10.7.0.1` 0% loss; `ping -M do -s 1400` returns a clear
"message too long / frag-needed" (never silent); iperf3 reports sensible
throughput; the TCPMSS rule shows a non-zero packet count; and the drop
counters confirm the data-plane isn't silently losing datagrams. Data travels
via QUIC DATAGRAMs only (no streams are opened).

Manual two-host run:

```sh
go build -tags phase0_insecure ./cmd/sieganet-server
go build -tags phase0_insecure ./cmd/sieganet-client

# server host
sudo ./sieganet-server -listen :4443 -tun-ip 10.7.0.1/24

# client host
sudo ./sieganet-client -server <server-ip>:4443 -tun-ip 10.7.0.2/24
ping 10.7.0.1
```

### Resilience details (Phase 0 hardening)

- **Drop counters.** Each side logs a `STATS` line on shutdown with separate
  counters for oversize drops, send errors, malformed datagrams, write-backlog
  drops and TUN-write errors. Comparing the client's `tx_sent` with the
  server's `rx_recv` reveals end-to-end datagram delivery; the kernel UDP
  `RcvbufErrors` counter (also printed) tells apart kernel-buffer loss from
  quic-go's internal 128-deep receive queue. At a >1 Gbit/s flood the inner TCP
  retransmits a small fraction — the counters show this is real path/queue loss,
  not phantom, and that the kernel buffers (raised to 16 MiB) are not the cause.
- **MTU gate.** The inner MTU is **not** a constant: each side probes the
  runtime maximum QUIC datagram size (via a no-wire `DatagramTooLargeError`
  probe) and clamps the TUN MTU to `InnerMTU(maxDatagram, padMax)` so a packet
  plus framing plus max padding always fits one datagram. If the path MTU
  shrinks mid-session the gate is lowered live and the TUN MTU re-clamped.
- **MSS clamp.** A `TCPMSS` rule pins forwarded TCP's SYN MSS to `innerMTU-40`,
  so PMTU-blackhole-prone middleboxes can't push oversized segments.
- **ICMP frag-needed.** An oversized IPv4 packet with DF set is answered with a
  crafted ICMP type 3 code 4 (next-hop MTU = inner MTU) injected back into the
  TUN, instead of a silent drop — so PMTU discovery converges.
- **`-rx-workers N`.** Number of goroutines draining quic-go's receive queue.
  Default 1 (ordering-safe). On fast, well-connected links raise it to cut
  queue-overflow drops (measured: 1→4 workers reduced self-inflicted drops
  ~2.5× at line rate) at the cost of possible reordering.
- **`siega-impair`** (`cmd/siega-impair`, test-only) is a userspace UDP relay
  that injects a fixed delay and random loss on the QUIC path, used by the test
  when the kernel has no `sch_netem`. Under delay 60ms + loss 2% the tunnel
  stays up (RTT ~122ms), inner TCP throughput collapses as expected for a lossy
  high-latency link, and PMTU/MSS keep working.

Unit tests (framing roundtrip, MTU-gate invariant, ICMP builder + checksums):

```sh
go test ./...
```
