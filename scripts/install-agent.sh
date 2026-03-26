#!/bin/bash
set -e

SERVER_URL="${OPSBOARD_SERVER:-http://192.168.10.65:3080}"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/opsboard"
BINARY="opsboard-agent"
ARCH=$(uname -m)

case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    *)       echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

echo "=== OpsBoard Agent Installer ==="
echo "Server: $SERVER_URL"
echo "Arch:   linux/$ARCH"

# Download binary
echo "[1/4] Downloading agent binary..."
curl -fsSL "$SERVER_URL/api/v1/agent/binary/linux/$ARCH" -o "/tmp/$BINARY"
chmod +x "/tmp/$BINARY"

# Verify checksum
echo "[2/4] Verifying checksum..."
EXPECTED=$(curl -fsSL "$SERVER_URL/api/v1/agent/binary/linux/$ARCH/sha256")
ACTUAL=$(sha256sum "/tmp/$BINARY" | awk '{print $1}')
if [ "$EXPECTED" != "$ACTUAL" ]; then
    echo "Checksum mismatch! Expected: $EXPECTED, Got: $ACTUAL"
    exit 1
fi

# Install
echo "[3/4] Installing..."
sudo mv "/tmp/$BINARY" "$INSTALL_DIR/$BINARY"
sudo mkdir -p "$CONFIG_DIR"

# Generate config if not exists
if [ ! -f "$CONFIG_DIR/agent.yaml" ]; then
    cat > /tmp/agent.yaml << EOF
server:
  address: "192.168.10.65:3101"
  token: "opsboard-dev-token"
  tls:
    enabled: false

collect:
  interval: 5
  docker: true
  gpu: false

agent:
  id: ""
EOF
    sudo mv /tmp/agent.yaml "$CONFIG_DIR/agent.yaml"
    echo "Created default config at $CONFIG_DIR/agent.yaml"
fi

# Create systemd service
echo "[4/4] Setting up systemd service..."
cat > /tmp/opsboard-agent.service << EOF
[Unit]
Description=OpsBoard Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/$BINARY -config $CONFIG_DIR/agent.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
sudo mv /tmp/opsboard-agent.service /etc/systemd/system/opsboard-agent.service
sudo systemctl daemon-reload
sudo systemctl enable opsboard-agent
sudo systemctl start opsboard-agent

echo ""
echo "=== Agent Installed Successfully ==="
echo "Config: $CONFIG_DIR/agent.yaml"
echo "Status: sudo systemctl status opsboard-agent"
echo "Logs:   sudo journalctl -u opsboard-agent -f"
