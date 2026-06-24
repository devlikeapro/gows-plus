#!/usr/bin/env bash
set -euo pipefail

GO_ROOT="$HOME/.local/go"
export PATH="$GO_ROOT/bin:$HOME/go/bin:$PATH"

if [ ! -x "$GO_ROOT/bin/go" ]; then
  mkdir -p "$HOME/.local"
  curl -fsSL https://go.dev/dl/go1.26.2.linux-amd64.tar.gz -o /tmp/go.tar.gz
  rm -rf "$GO_ROOT"
  tar -C "$HOME/.local" -xzf /tmp/go.tar.gz
fi

if ! pkg-config --exists vips 2>/dev/null; then
  sudo apt-get update -qq
  sudo apt-get install -y -qq build-essential libvips-dev protobuf-compiler pkg-config
fi

go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
make build-proto
cd src
go mod tidy
go build -o ../bin/gows .
go test ./voip/...
