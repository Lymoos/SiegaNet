#!/usr/bin/env bash
# Phase 1, step 2 verification: the static decoy front is byte-for-byte identical
# to a stock Go http.FileServer over the same site — same header set AND order,
# same body, including the 404 — so an active prober cannot tell the endpoint
# apart from a plain static website. The ONLY header the front adds is Alt-Svc
# (the intentional HTTP/3 advertisement).
#
# It runs sieganet-server (static decoy over internal/decoy/site) and a reference
# http.FileServer over the same directory, then diffs raw responses (unsorted, so
# header ORDER is checked) and lists any header present only on the front.
#
# Requires openssl, curl, go. Runs on localhost; no root.
set -euo pipefail

DIR="$(mktemp -d)"
SITE="internal/decoy/site"
SRV_PID="" REF_PID=""
cleanup() { [ -n "$SRV_PID" ] && kill "$SRV_PID" 2>/dev/null; [ -n "$REF_PID" ] && kill "$REF_PID" 2>/dev/null; rm -rf "$DIR"; }
trap cleanup EXIT

DOMAIN="siega.test"
FRONT="https://$DOMAIN:8443"
REF="http://127.0.0.1:8080"
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
domain     = "$DOMAIN"
listen_tcp = "127.0.0.1:8443"
listen_udp = "127.0.0.1:8443"
cert_mode  = "file"
cert_file  = "$DIR/fullchain.crt"
key_file   = "$DIR/leaf.key"
decoy_mode = "static"
decoy_dir  = "$SITE"
EOF

echo ">> building sieganet-server and a reference http.FileServer"
go build -o "$DIR/server" ./cmd/sieganet-server
cat >"$DIR/ref.go" <<'EOF'
package main
import ("net/http"; "os")
func main(){ panic(http.ListenAndServe(os.Args[1], http.FileServer(http.Dir(os.Args[2])))) }
EOF

"$DIR/server" -config "$DIR/server.toml" & SRV_PID=$!
go run "$DIR/ref.go" 127.0.0.1:8080 "$SITE" & REF_PID=$!
sleep 1.5

fetch_front() { curl -s --http1.1 --cacert "$DIR/ca.crt" --resolve "$DOMAIN:8443:127.0.0.1" -D "$1" -o "$2" "$FRONT$3"; }
fetch_ref()   { curl -s --http1.1 -D "$1" -o "$2" "$REF$3"; }
# strip CR, Date and Alt-Svc (the only intentional front-only header)
strip()   { tr -d '\r' <"$1" | grep -iv -e '^date:' -e '^alt-svc:' | sed '/^$/d'; }
hnames()  { tr -d '\r' <"$1" | sed -n 's/^\([A-Za-z-]*\):.*/\1/p' | tr 'A-Z' 'a-z' | grep -iv '^date$' | sort -u; }

echo
echo "==================================================================="
echo "  FRONT (TLS, static decoy) vs REFERENCE http.FileServer"
echo "  unsorted diff => proves identical header SET *and ORDER* + body"
echo "==================================================================="
for p in / /about.html /journal.html /style.css /favicon.svg /robots.txt /this-path-does-not-exist; do
  fetch_front "$DIR/fh" "$DIR/fb" "$p"
  fetch_ref   "$DIR/rh" "$DIR/rb" "$p"
  hdr_diff="$(diff <(strip "$DIR/fh") <(strip "$DIR/rh") || true)"
  body_diff="$(cmp -s "$DIR/fb" "$DIR/rb" && echo "" || echo "BODY DIFFERS")"
  status="$(head -1 "$DIR/fh" | tr -d '\r')"
  if [ -z "$hdr_diff" ] && [ -z "$body_diff" ]; then
    printf "  %-26s %-24s IDENTICAL (order+body)\n" "$p" "[$status]"
  else
    printf "  %-26s %-24s MISMATCH\n" "$p" "[$status]"; FAIL=1
    [ -n "$hdr_diff" ] && { echo "    --- header diff (front vs ref) ---"; echo "$hdr_diff" | sed 's/^/    /'; }
    [ -n "$body_diff" ] && echo "    $body_diff"
  fi
done

echo
echo ">> Headers present ONLY on the front (expect exactly: alt-svc):"
fetch_front "$DIR/fh" /dev/null "/"
fetch_ref   "$DIR/rh" /dev/null "/"
only_front="$(comm -23 <(hnames "$DIR/fh") <(hnames "$DIR/rh"))"
echo "    ${only_front:-<none>}"
[ "$only_front" = "alt-svc" ] || { echo "    !! unexpected front-only header(s)"; FAIL=1; }

echo
echo "==================================================================="
echo "  Raw FRONT 404 (canonical FileServer body+order, plus Alt-Svc):"
echo "==================================================================="
fetch_front "$DIR/fh" "$DIR/fb" "/this-path-does-not-exist"
tr -d '\r' <"$DIR/fh" | sed 's/^/    /'
echo "    body: $(cat "$DIR/fb")"

echo
echo "==================================================================="
echo "  Weird requests behave like the reference web server"
echo "==================================================================="
weird() { # <label> <curl-args...>   on /about.html (a directly-served file)
  local label="$1"; shift
  local f r
  f="$(curl -s -o /dev/null --http1.1 --cacert "$DIR/ca.crt" --resolve "$DOMAIN:8443:127.0.0.1" -w '%{http_code}' "$@" "$FRONT/about.html" || echo ERR)"
  r="$(curl -s -o /dev/null --http1.1 -w '%{http_code}' "$@" "$REF/about.html" || echo ERR)"
  printf "  %-34s front=%s ref=%s %s\n" "$label" "$f" "$r" "$([ "$f" = "$r" ] && echo OK || { echo MISMATCH; FAIL=1; })"
}
weird "bad Range (bytes=99999999-)"  -H "Range: bytes=99999999-"
weird "valid Range (bytes=0-9)"      -H "Range: bytes=0-9"
weird "weird method (FROBNICATE)"    -X FROBNICATE
weird "DELETE method"                -X DELETE

echo
echo ">> HTTP/0.9 / malformed request line (raw 'GET /' with no version):"
echo "   front  (openssl s_client):"
printf 'GET /\r\n' | timeout 3 openssl s_client -quiet -connect 127.0.0.1:8443 -servername "$DOMAIN" 2>/dev/null | head -1 | sed 's/^/     /' || true
echo "   ref    (raw TCP):"
printf 'GET /\r\n' | timeout 3 bash -c 'exec 3<>/dev/tcp/127.0.0.1/8080; cat >&3; head -1 <&3' 2>/dev/null | sed 's/^/     /' || true
echo "   (Go answers a normal 400 Bad Request to a malformed line — no custom error, no panic)"

echo
if [ "$FAIL" = 0 ]; then
  echo ">> RESULT: front is byte-identical to a stock FileServer (order+body); only extra header is Alt-Svc."
else
  echo ">> RESULT: MISMATCH found (see above)."; exit 1
fi
