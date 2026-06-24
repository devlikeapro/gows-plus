#!/usr/bin/env bash
set -euo pipefail

GO_ROOT="$HOME/.local/go"
export PATH="$GO_ROOT/bin:$HOME/go/bin:$PATH"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
make build-proto
cd src
go test ./voip/...
