#!/bin/bash
set -e

CERT_DIR="${1:-/opt/opsboard/certs}"
mkdir -p "$CERT_DIR"

echo "=== Generating Self-Signed CA and Server Certificate ==="

# Generate CA key and cert
openssl genrsa -out "$CERT_DIR/ca.key" 4096
openssl req -new -x509 -key "$CERT_DIR/ca.key" -sha256 \
    -subj "/CN=OpsBoard CA" -days 3650 \
    -out "$CERT_DIR/ca.crt"

# Generate server key
openssl genrsa -out "$CERT_DIR/server.key" 2048

# Generate server CSR
openssl req -new -key "$CERT_DIR/server.key" \
    -subj "/CN=opsboard-server" \
    -out "$CERT_DIR/server.csr"

# Sign server cert with CA
cat > "$CERT_DIR/server.ext" << EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage=digitalSignature,keyEncipherment
subjectAltName=DNS:opsboard-server,DNS:localhost,IP:127.0.0.1,IP:192.168.10.65
EOF

openssl x509 -req -in "$CERT_DIR/server.csr" \
    -CA "$CERT_DIR/ca.crt" -CAkey "$CERT_DIR/ca.key" \
    -CAcreateserial -days 365 -sha256 \
    -extfile "$CERT_DIR/server.ext" \
    -out "$CERT_DIR/server.crt"

# Clean up
rm -f "$CERT_DIR/server.csr" "$CERT_DIR/server.ext" "$CERT_DIR/ca.srl"

echo ""
echo "=== Certificates Generated ==="
echo "CA cert:     $CERT_DIR/ca.crt (distribute to cloud Agent)"
echo "Server cert: $CERT_DIR/server.crt"
echo "Server key:  $CERT_DIR/server.key"
echo ""
echo "For cloud Agent, copy ca.crt to /etc/opsboard/ca.crt"
