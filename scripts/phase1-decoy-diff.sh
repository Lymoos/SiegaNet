#!/usr/bin/env bash
# Phase 1, step 2 verification: the public endpoint transparently relays every
# non-tunnel request to the real backend site. We prove it by diffing, byte for
# byte, the front (sieganet-server over TLS) against the backend served directly
# — for normal pages AND unknown paths — and by probing weird requests.
#
# The only intentional difference is the Date header (regenerated) and the
# Alt-Svc header (the front advertises HTTP/3, like a real H3 site); both are
# stripped before diffing.
#
# Requires openssl, curl. Runs on localhost; no root needed.
set -euo pipefail

DIR="$(mktemp -d)"
SRV_PID=""
cleanup() { [ -n "$SRV_PID" ] && kill "$SRV_PID" 2>/dev/null; rm -rf "$DIR"; }
trap cleanup EXIT

DOMAIN="siega.test"
FRONT="https://$DOMAIN:8443"
BACKEND="http://127.0.0.1:8080"
FAIL=0

echo ">> generating throwaway CA + leaf"
openssl ecparam -name prime256v1 -genkey -noout -out "$DIR/ca.key" 2>/dev/null
openssl req -x509 -new -key "$DIR/ca.key" -sha256 -days 7 -subj "/CN=SiegaNet Test CA" -out "$DIR/ca.crt" 2>/dev/null
openssl ecparam -name prime256v1 -genkey -noout -out "$DIR/leaf.key" 2>/dev/null
openssl req -new -key "$DIR/leaf.key" -subj "/CN=$DOMAIN" -out "$DIR/leaf.csr" 2>/dev/null
printf 'subjectAltName=DNS:%s,IP:127.0.0.1\nextendedKeyUsage=serverAuth\n' "$DOMAIN" >"$DIR/ext.cnf"
openssl x509 -req -in "$DIR/leaf.csr" -CA "$DIR/ca.crt" -CAkey "$DIR/ca.key" -CAcreateserial \
  -days 7 -sha256 -extfile "$DIR/ext.cnf" -out "$DIR/leaf.crt" 2>/dev/null
cat "$DIR/leaf.crt" "$DIR/ca.crt" >"$DIR/fullchain.crt"

cat >"$DIR/server.toml" <<EOF
domain               = "$DOMAIN"
listen_tcp           = "127.0.0.1:8443"
listen_udp           = "127.0.0.1:8443"
cert_mode            = "file"
cert_file            = "$DIR/fullchain.crt"
key_file             = "$DIR/leaf.key"
decoy_mode           = "static"
decoy_backend_listen = "127.0.0.1:8080"
EOF

echo ">> starting sieganet-server (static decoy, backend pinned to 127.0.0.1:8080)"
go build -o "$DIR/server" ./cmd/sieganet-server
"$DIR/server" -config "$DIR/server.toml" &
SRV_PID=$!
sleep 1

# fetch <front|backend> <path> <hdr-out> <body-out>
fetch_front()   { curl -s --http1.1 --cacert "$DIR/ca.crt" --resolve "$DOMAIN:8443:127.0.0.1" -D "$1" -o "$2" "$FRONT$3"; }
fetch_backend() { curl -s --http1.1 -D "$1" -o "$2" "$BACKEND$3"; }
# normalise for comparison: keep the status line, then strip Date/Alt-Svc and the
# blank separator and sort the remaining headers (a reverse proxy may legitimately
# reorder headers; what matters is identical status + header set + values, and the
# order is invisible to a passive observer inside TLS anyway).
norm() {
  tr -d '\r' <"$1" >"$1.tmp"
  head -1 "$1.tmp"
  tail -n +2 "$1.tmp" | grep -iv -e '^date:' -e '^alt-svc:' | sed '/^$/d' | sort
}

echo
echo "==================================================================="
echo "  curl-diff: FRONT (TLS relay) vs BACKEND (direct) per path"
echo "==================================================================="
for p in / /about.html /journal.html /style.css /favicon.svg /robots.txt /this-path-does-not-exist; do
  fetch_front   "$DIR/fh" "$DIR/fb" "$p"
  fetch_backend "$DIR/bh" "$DIR/bb" "$p"
  hdr_diff="$(diff <(norm "$DIR/fh") <(norm "$DIR/bh") || true)"
  body_diff="$(cmp -s "$DIR/fb" "$DIR/bb" && echo "" || echo "BODY DIFFERS")"
  status="$(head -1 "$DIR/fh" | tr -d '\r')"
  if [ -z "$hdr_diff" ] && [ -z "$body_diff" ]; then
    printf "  %-26s %-18s IDENTICAL (headers+body)\n" "$p" "[$status]"
  else
    printf "  %-26s %-18s MISMATCH\n" "$p" "[$status]"
    [ -n "$hdr_diff" ] && { echo "    --- header diff ---"; echo "$hdr_diff" | sed 's/^/    /'; }
    [ -n "$body_diff" ] && echo "    $body_diff"
    FAIL=1
  fi
done

echo
echo "==================================================================="
echo "  Sample raw front responses (what a prober actually sees)"
echo "==================================================================="
echo ">> FRONT GET /  (normal page, headers):"
fetch_front "$DIR/fh" "$DIR/fb" "/"; norm "$DIR/fh" | sed 's/^/    /'
echo "    <body: $(wc -c <"$DIR/fb") bytes of the real homepage>"
echo
echo ">> FRONT GET /this-path-does-not-exist  (unknown path -> backend's 404, not ours):"
fetch_front "$DIR/fh" "$DIR/fb" "/this-path-does-not-exist"; norm "$DIR/fh" | sed 's/^/    /'
echo "    body: $(cat "$DIR/fb")"

echo
echo "==================================================================="
echo "  Weird requests behave like a normal web server (front vs backend)"
echo "==================================================================="
weird() { # <label> <curl-args...>   (uses /about.html: a directly-served file)
  local label="$1"; shift
  local f b
  f="$(curl -s -o /dev/null --http1.1 --cacert "$DIR/ca.crt" --resolve "$DOMAIN:8443:127.0.0.1" -w '%{http_code}' "$@" "$FRONT/about.html" || echo ERR)"
  b="$(curl -s -o /dev/null --http1.1 -w '%{http_code}' "$@" "$BACKEND/about.html" || echo ERR)"
  printf "  %-34s front=%s backend=%s %s\n" "$label" "$f" "$b" "$([ "$f" = "$b" ] && echo OK || { echo MISMATCH; FAIL=1; })"
}
weird "bad Range (bytes=99999999-)"  -H "Range: bytes=99999999-"
weird "valid Range (bytes=0-9)"      -H "Range: bytes=0-9"
weird "weird method (FROBNICATE)"    -X FROBNICATE
weird "DELETE method"                -X DELETE

echo
echo ">> HTTP/0.9 / malformed request line (raw 'GET /' with no version):"
echo "   front  (openssl s_client):"
printf 'GET /\r\n' | timeout 3 openssl s_client -quiet -connect 127.0.0.1:8443 -servername "$DOMAIN" 2>/dev/null | head -1 | sed 's/^/     /' || true
echo "   backend (raw TCP):"
printf 'GET /\r\n' | timeout 3 bash -c 'exec 3<>/dev/tcp/127.0.0.1/8080; cat >&3; head -1 <&3' 2>/dev/null | sed 's/^/     /' || true
echo "   (Go's HTTP server answers a normal 400 Bad Request to a malformed line — no custom error, no panic)"

echo
if [ "$FAIL" = 0 ]; then
  echo ">> RESULT: front == backend for every path (excluding Date/Alt-Svc). Relay is transparent."
else
  echo ">> RESULT: MISMATCH found (see above)."; exit 1
fi
