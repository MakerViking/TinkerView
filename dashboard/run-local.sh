#!/usr/bin/env bash
# Build the SPA, embed it into the Go binary, and run the dashboard locally in
# DEMO mode (synthetic printer + camera tiles) on http://127.0.0.1:8088.
#
# Requires Go 1.23+ and Node on PATH. No Go? Install without root:
#   curl -fsSL https://go.dev/dl/go1.23.4.linux-amd64.tar.gz | tar -C "$HOME/.local" -xz
#   export PATH="$HOME/.local/go/bin:$PATH"
set -euo pipefail
cd "$(dirname "$0")"

echo "==> building frontend (npm)"
( cd web && npm install --no-audit --no-fund && npm run build )

echo "==> embedding SPA into the Go binary"
rm -rf internal/web/dist
cp -r web/dist internal/web/dist

echo "==> go build"
go build -trimpath -ldflags "-X main.Version=local" -o ./bin/dashboard ./cmd/dashboard

echo "==> running on http://127.0.0.1:8088  (Ctrl-C to stop)"
DASHBOARD_DEMO=1 DASHBOARD_LISTEN=:8088 ./bin/dashboard -config ./config.yaml
