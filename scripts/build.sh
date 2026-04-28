#!/bin/bash
# Build MC-Starter for Windows only
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="${1:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}"
LDFLAGS="-s -w -X main.version=${VERSION}"

echo "==> go mod tidy"
go mod tidy

echo "==> building starter ${VERSION} for windows/amd64"
GOOS=windows GOARCH=amd64 go build -ldflags="${LDFLAGS}" -o build/starter.exe ./cmd/starter/

echo "==> done: build/starter.exe ($(ls -lh build/starter.exe | awk '{print $5}'))"

echo ""
echo "to build release (no console window):"
echo "  GOOS=windows GOARCH=amd64 go build -ldflags=\"${LDFLAGS} -H windowsgui\" -o build/starter-${VERSION}-x64.exe ./cmd/starter/"
