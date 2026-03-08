#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$ROOT_DIR/build/internal"

mkdir -p "$OUT_DIR"
go build -ldflags "-X main.BuildEdition=internal" -o "$OUT_DIR/groot-internal" ./cmd/groot-api
