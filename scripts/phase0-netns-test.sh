#!/usr/bin/env bash
# Phase 0 acceptance + resilience test.
#
# Two network namespaces joined by a veth pair run the SiegaNet server and
# client. We verify the data-plane on a clean link and again under a mobile-like
# impairment (delay 60ms + loss 2%), and check that oversized DF packets do not
# vanish silently (PMTU). After each pass we print the data-plane drop counters
# so a high inner-TCP retransmit count can be attributed to path loss vs. our
# own datagram drops.
#
# The impaired pass uses tc-netem when sch_netem is available, otherwise it
# falls back to the userspace siega-impair UDP relay (works in any kernel).
#
# Requires root, iproute2 (ip), iperf3, iputils-ping, iptables (and tc/sch_netem
# for the netem path; the relay fallback needs none of those).
set -euo pipefail

NS_SRV=siega-srv
NS_CLI=siega-cli
BIN_DIR="$(mktemp -d)"
SRV_OUT="$(mktemp)"
CLI_OUT="$(mktemp)"
RELAY_OUT="$(mktemp)"
SRV_PID="" CLI_PID="" RELAY_PID=""
DIAL_ADDR="10.0.0.1:4443"   # what the client dials; overridden when relaying

cleanup() {
  set +e
  [ -n "$CLI_PID" ] && kill -TERM "$CLI_PID" 2>/dev/null
  [ -n "$SRV_PID" ] && kill -TERM "$SRV_PID" 2>/dev/null
  [ -n "$RELAY_PID" ] && kill -TERM "$RELAY_PID" 2>/dev/null
  ip netns del "$NS_CLI" 2>/dev/null
  ip netns del "$NS_SRV" 2>/dev/null
  rm -rf "$BIN_DIR" "$SRV_OUT" "$CLI_OUT" "$RELAY_OUT"
}
trap cleanup EXIT

stat_field() { # <logfile> <field>  -> value from the STATS line
  grep "STATS" "$1" | tail -1 | grep -o "$2=[0-9]*" | cut -d= -f2
}

# srv_udp <field-index> reads a counter from the server namespace's UDP MIB.
# Fields: 1=InDatagrams 3=InErrors 5=RcvbufErrors (1-based after the label).
srv_udp() {
  ip netns exec "$NS_SRV" awk -v i="$1" '/^Udp: [0-9]/{print $(i+1)}' /proc/net/snmp
}

start_tunnel() {
  : >"$SRV_OUT"; : >"$CLI_OUT"
  ip netns exec "$NS_SRV" "$BIN_DIR/server" \
    -listen 10.0.0.1:4443 -tun siega0 -tun-ip 10.7.0.1/24 >"$SRV_OUT" 2>&1 &
  SRV_PID=$!
  sleep 1.5
  ip netns exec "$NS_CLI" "$BIN_DIR/client" \
    -server "$DIAL_ADDR" -tun siega0 -tun-ip 10.7.0.2/24 >"$CLI_OUT" 2>&1 &
  CLI_PID=$!
  sleep 2
}

stop_tunnel() {
  kill -TERM "$CLI_PID" 2>/dev/null; kill -TERM "$SRV_PID" 2>/dev/null
  sleep 1.2          # let both flush their STATS line on shutdown
  CLI_PID="" SRV_PID=""
}

run_pass() { # <label>
  local label="$1"
  echo
  echo "############################################################"
  echo "## PASS: $label"
  echo "############################################################"
  start_tunnel

  echo ">> ping 10.7.0.1 through the tunnel"
  ip netns exec "$NS_CLI" ping -c 5 -W 2 10.7.0.1 || echo "(some loss/latency expected under impairment)"

  echo
  echo ">> PMTU: ping -M do -s 1400 (1428B, DF) must NOT vanish silently"
  ip netns exec "$NS_CLI" ping -M do -s 1400 -c 2 -W 2 10.7.0.1 || true
  echo "   ^ either replies, or a clear 'message too long'/frag-needed — not silence"

  echo
  echo ">> iperf3 through the tunnel"
  local ierr0 irbuf0 ierr1 irbuf1
  ierr0="$(srv_udp 3)"; irbuf0="$(srv_udp 5)"
  ip netns exec "$NS_SRV" iperf3 -s -1 -B 10.7.0.1 >/dev/null 2>&1 &
  local ipid=$!
  sleep 1
  ip netns exec "$NS_CLI" iperf3 -c 10.7.0.1 -t 5 || echo "(throughput drops under loss — expected)"
  kill "$ipid" 2>/dev/null || true
  ierr1="$(srv_udp 3)"; irbuf1="$(srv_udp 5)"

  echo
  echo ">> MSS clamp rule (client OUTPUT mangle) — pkts column proves it matched the SYN"
  ip netns exec "$NS_CLI" iptables -t mangle -L OUTPUT -n -v | grep -E "TCPMSS|Chain" || true

  stop_tunnel

  echo
  echo ">> data-plane counters ($label):"
  grep -E "mtu gate|udp socket buffers|STATS|kernel drops" "$SRV_OUT" | sed 's/^/   [srv] /'
  grep -E "mtu gate|udp socket buffers|STATS|kernel drops" "$CLI_OUT" | sed 's/^/   [cli] /'

  local tx rx
  tx="$(stat_field "$CLI_OUT" tx_sent)"; rx="$(stat_field "$SRV_OUT" rx_recv)"
  if [ -n "$tx" ] && [ -n "$rx" ] && [ "$tx" -gt 0 ]; then
    echo "   delivery (client tx_sent=$tx -> server rx_recv=$rx): $(( rx * 100 / tx ))% of upstream datagrams arrived"
    echo "   (a gap here = datagrams lost on the path/buffers/quic-go rcv queue, NOT inner-TCP retransmits)"
  fi
  echo "   server kernel UDP during iperf: InErrors+=$(( ierr1 - ierr0 )) RcvbufErrors+=$(( irbuf1 - irbuf0 ))"
  echo "   (RcvbufErrors==0 with a delivery gap means the kernel buffer was fine and the"
  echo "    loss was quic-go's internal 128-deep datagram queue, not SO_RCVBUF — raise -rx-workers)"
}

echo ">> raising UDP socket buffer limits (sysctl)"
sysctl -w net.core.rmem_max=16777216 net.core.wmem_max=16777216 >/dev/null

echo ">> building binaries (-tags phase0_insecure)"
go build -tags phase0_insecure -o "$BIN_DIR/server" ./cmd/sieganet-server
go build -tags phase0_insecure -o "$BIN_DIR/client" ./cmd/sieganet-client
go build -o "$BIN_DIR/impair" ./cmd/siega-impair

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

# ---- PASS 1: clean link ----
run_pass "clean link"

# ---- PASS 2: mobile-like impairment (delay 60ms + loss 2%) ----
# Prefer tc-netem; if the kernel lacks sch_netem, fall back to the userspace
# siega-impair UDP relay, which injects the same delay/loss on the QUIC path.
echo
if ip netns exec "$NS_CLI" tc qdisc add dev veth-cli root netem delay 60ms loss 2% 2>/dev/null; then
  echo ">> impairment via tc-netem (delay 60ms loss 2%, each direction)"
  ip netns exec "$NS_SRV" tc qdisc add dev veth-srv root netem delay 60ms loss 2%
  run_pass "tc-netem delay 60ms loss 2%"
  ip netns exec "$NS_CLI" tc qdisc del dev veth-cli root 2>/dev/null
  ip netns exec "$NS_SRV" tc qdisc del dev veth-srv root 2>/dev/null
else
  echo ">> sch_netem unavailable in this kernel — using userspace siega-impair relay instead"
  ip netns exec "$NS_SRV" "$BIN_DIR/impair" \
    -listen 10.0.0.1:4444 -target 10.0.0.1:4443 -delay 60ms -loss 0.02 >"$RELAY_OUT" 2>&1 &
  RELAY_PID=$!
  sleep 0.5
  DIAL_ADDR="10.0.0.1:4444"   # client dials the relay
  run_pass "siega-impair delay 60ms loss 2% (each direction)"
  DIAL_ADDR="10.0.0.1:4443"
  kill -TERM "$RELAY_PID" 2>/dev/null; RELAY_PID=""
  echo ">> relay impairment summary:"
  grep -E "relaying|relayed=" "$RELAY_OUT" | tail -3 | sed 's/^/   /'
fi

echo
echo ">> ALL PHASE 0 PASSES COMPLETED"
