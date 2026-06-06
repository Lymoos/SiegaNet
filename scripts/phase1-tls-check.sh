#!/usr/bin/env bash
# Phase 1, step 1 manual check: the server presents a valid certificate chain
# over TCP (HTTP/1.1+HTTP/2) and QUIC (HTTP/3) on port 443.
#
# Generates a throwaway CA + leaf with openssl, runs sieganet-server in
# cert_mode=file, then verifies the chain with curl (--cacert) and
# openssl s_client. HTTP/3 is verified by the Go integration test
# (internal/server) because the system curl here lacks http3 support.
#
# Requires openssl, curl. Runs on localhost:8443; no root needed.
set -euo pipefail

DIR="$(mktemp -d)"
SRV_PID=""
cleanup() { [ -n "$SRV_PID" ] && kill "$SRV_PID" 2>/dev/null; rm -rf "$DIR"; }
trap cleanup EXIT

DOMAIN="siega.test"

echo ">> generating throwaway CA + leaf for $DOMAIN"
openssl ecparam -name prime256v1 -genkey -noout -out "$DIR/ca.key" 2>/dev/null
openssl req -x509 -new -key "$DIR/ca.key" -sha256 -days 7 \
  -subj "/CN=SiegaNet Test CA" -out "$DIR/ca.crt" 2>/dev/null
openssl ecparam -name prime256v1 -genkey -noout -out "$DIR/leaf.key" 2>/dev/null
openssl req -new -key "$DIR/leaf.key" -subj "/CN=$DOMAIN" -out "$DIR/leaf.csr" 2>/dev/null
cat >"$DIR/ext.cnf" <<EOF
subjectAltName = DNS:$DOMAIN, IP:127.0.0.1
extendedKeyUsage = serverAuth
EOF
openssl x509 -req -in "$DIR/leaf.csr" -CA "$DIR/ca.crt" -CAkey "$DIR/ca.key" \
  -CAcreateserial -days 7 -sha256 -extfile "$DIR/ext.cnf" -out "$DIR/leaf.crt" 2>/dev/null
# Server presents leaf + CA chain.
cat "$DIR/leaf.crt" "$DIR/ca.crt" >"$DIR/fullchain.crt"

cat >"$DIR/server.toml" <<EOF
domain     = "$DOMAIN"
listen_tcp = "127.0.0.1:8443"
listen_udp = "127.0.0.1:8443"
cert_mode  = "file"
cert_file  = "$DIR/fullchain.crt"
key_file   = "$DIR/leaf.key"
EOF

echo ">> building and starting sieganet-server (cert_mode=file)"
go build -o "$DIR/server" ./cmd/sieganet-server
"$DIR/server" -config "$DIR/server.toml" &
SRV_PID=$!
sleep 1

echo
echo ">> curl over TCP, verifying the chain against the CA (--cacert), HTTP/2:"
curl -sS --cacert "$DIR/ca.crt" --resolve "$DOMAIN:8443:127.0.0.1" \
  -w "  [http_version=%{http_version} response_code=%{response_code} ssl_verify=%{ssl_verify_result}]\n" \
  "https://$DOMAIN:8443/"

echo
echo ">> curl forcing HTTP/1.1:"
curl -sS --http1.1 --cacert "$DIR/ca.crt" --resolve "$DOMAIN:8443:127.0.0.1" \
  -w "  [http_version=%{http_version} response_code=%{response_code}]\n" -o /dev/null \
  "https://$DOMAIN:8443/"

echo
echo ">> openssl s_client chain verification (Verify return code: 0 == OK):"
echo | openssl s_client -connect 127.0.0.1:8443 -servername "$DOMAIN" \
  -CAfile "$DIR/ca.crt" 2>/dev/null | grep -E "Verify return code|subject=|issuer=|Protocol|ALPN"

echo
echo ">> NOTE: HTTP/3 chain verification is covered by 'go test ./internal/server'"
echo ">> (system curl lacks http3). In production cert_mode=acme yields a"
echo ">> browser-trusted Let's Encrypt cert (real padlock)."
