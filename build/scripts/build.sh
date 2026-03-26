#!/bin/bash
set -e

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_DIR="$PROJECT_DIR/build"

echo "=== OpsBoard Build Script ==="

# Clean
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

# Build frontend
echo "[1/3] Building frontend..."
cd "$PROJECT_DIR/web"
npm ci
npm run build
cp -r dist "$BUILD_DIR/web-dist"

# Build server (for local testing, Docker builds its own)
echo "[2/3] Building server..."
cd "$PROJECT_DIR/server"
CGO_ENABLED=1 go build -o "$BUILD_DIR/opsboard-server" ./cmd/server/

# Build agent (cross-compile for Linux)
echo "[3/3] Building agent..."
cd "$PROJECT_DIR/agent"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$BUILD_DIR/opsboard-agent-linux-amd64" ./cmd/agent/

echo ""
echo "=== Build Complete ==="
echo "Frontend:  $BUILD_DIR/web-dist/"
echo "Server:    $BUILD_DIR/opsboard-server"
echo "Agent:     $BUILD_DIR/opsboard-agent-linux-amd64"
ls -lh "$BUILD_DIR/"
