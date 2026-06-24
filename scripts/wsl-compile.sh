#!/usr/bin/env bash
set -euo pipefail
export PATH="$HOME/.local/go/bin:$HOME/go/bin:$PATH"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT/src"
go build -o ../bin/gows .
