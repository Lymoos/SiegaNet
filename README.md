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

Phase 0 is a bare QUIC↔TUN bridge: self-signed TLS, no auth, no masking. It
proves the transport is alive. Requires Linux, root, iproute2, iperf3.

Automated acceptance test (two network namespaces joined by a veth pair —
runs ping + iperf3 through the tunnel):

```sh
sudo bash scripts/phase0-netns-test.sh
```

Expected: `ping 10.7.0.1` succeeds with 0% loss and iperf3 reports a sensible
throughput. Data travels via QUIC DATAGRAMs only (no streams are opened).

Manual two-host run:

```sh
# server host
sudo ./sieganet-server -listen :4443 -tun-ip 10.7.0.1/24

# client host
sudo ./sieganet-client -server <server-ip>:4443 -tun-ip 10.7.0.2/24
ping 10.7.0.1
```

Unit tests:

```sh
go test ./...
```
