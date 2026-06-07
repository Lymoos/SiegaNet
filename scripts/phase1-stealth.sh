#!/usr/bin/env bash
# Phase 1, step 4 verification (hands-on): the WebTransport tunnel on the magic
# path, the auth edge cases, full-response indistinguishability, and the timing
# distributions (p50/p90/p99 + histogram) for magic-path probing vs a generic
# 404. Everything runs in-process so the timing has microsecond resolution and
# no network jitter.
#
# Requires go. No root.
set -euo pipefail
cd "$(dirname "$0")/.."

echo ">> WebTransport + auth + indistinguishability (full response):"
go test ./internal/server/ -run 'TestValidPeerOpensTunnelAndEchoes|TestAuthEdgeCasesOnMagicPath|TestMagicPathProbeLooksLikeGeneric404' -v -count=1

echo
echo ">> Transport-agnostic data-plane (pump over a non-QUIC Datagrammer):"
go test ./internal/tunnel/ -run 'TestPumpOverMemTransport' -v -count=1

echo
echo ">> Timing distributions: magic-path probe vs generic 404 (4000 samples each):"
go test ./internal/server/ -run 'TestTimingParityMagicVsGeneric' -v -count=1
