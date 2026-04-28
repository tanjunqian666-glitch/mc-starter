#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")/.."

echo "==> go mod tidy"
go mod tidy

echo "==> building for current platform"
go build -o build/starter ./cmd/starter/

echo "==> done: build/starter"
