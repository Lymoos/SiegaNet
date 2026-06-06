#!/usr/bin/env bash
# Phase 0 acceptance test: prove the QUIC<->TUN data-plane works.
#
# Sets up two network namespaces joined by a veth pair, runs the SiegaNet
# server in one and the client in the other, then verifies that an IP packet
# travels through the tunnel (ping) and measures throughput (iperf3).
#
# Requires root, iproute2 and iperf3. Run from the repo root.
set -euo pipefail

NS_SRV=siega-srv
NS_CLI=siega-cli
BIN_DIR="$(mktemp -d)"
SRV_OUT="$(mktemp)"
CLI_OUT="$(mktemp)"
SRV_PID=""
CLI_PID=""

cleanup() {
  set +e
  [ -n "$CLI_PID" ] && kill "$CLI_PID" 2>/dev/null
  [ -n "$SRV_PID" ] && kill "$SRV_PID" 2>/dev/null
  ip netns del "$NS_CLI" 2>/dev/null
  ip netns del "$NS_SRV" 2>/dev/null
  rm -rf "$BIN_DIR"
  echo
  echo "=== server log ==="; cat "$SRV_OUT"
  echo "=== client log ==="; cat "$CLI_OUT"
  rm -f "$SRV_OUT" "$CLI_OUT"
}
trap cleanup EXIT

echo ">> building binaries"
go build -o "$BIN_DIR/server" ./cmd/sieganet-server
go build -o "$BIN_DIR/client" ./cmd/sieganet-client

echo ">> creating namespaces and veth"
ip netns add "$NS_SRV"
ip netns add "$NS_CLI"
ip link add veth-srv netns "$NS_SRV" type veth peer name veth-cli netns "$NS_CLI"
ip -n "$NS_SRV" addr add 10.0.0.1/24 dev veth-srv
ip -n "$NS_CLI" addr add 10.0.0.2/24 dev veth-cli
ip -n "$NS_SRV" link set veth-srv up
ip -n "$NS_CLI" link set veth-cli up
ip -n "$NS_SRV" link set lo up
ip -n "$NS_CLI" link set lo up

echo ">> starting server in $NS_SRV"
ip netns exec "$NS_SRV" "$BIN_DIR/server" \
  -listen 10.0.0.1:4443 -tun siega0 -tun-ip 10.7.0.1/24 >"$SRV_OUT" 2>&1 &
SRV_PID=$!
sleep 1.5

echo ">> starting client in $NS_CLI"
ip netns exec "$NS_CLI" "$BIN_DIR/client" \
  -server 10.0.0.1:4443 -tun siega0 -tun-ip 10.7.0.2/24 >"$CLI_OUT" 2>&1 &
CLI_PID=$!
sleep 2

echo
echo ">> TEST 1: ping 10.7.0.1 (server inner IP) through the tunnel"
if ip netns exec "$NS_CLI" ping -c 4 -W 2 10.7.0.1; then
  echo "PING OK"
else
  echo "PING FAILED"; exit 1
fi

echo
echo ">> TEST 2: iperf3 throughput through the tunnel"
ip netns exec "$NS_SRV" iperf3 -s -1 -B 10.7.0.1 >/dev/null 2>&1 &
IPERF_PID=$!
sleep 1
if ip netns exec "$NS_CLI" iperf3 -c 10.7.0.1 -t 5; then
  echo "IPERF OK"
else
  echo "IPERF FAILED"; kill "$IPERF_PID" 2>/dev/null; exit 1
fi
kill "$IPERF_PID" 2>/dev/null || true

echo
echo ">> ALL PHASE 0 ACCEPTANCE TESTS PASSED"
