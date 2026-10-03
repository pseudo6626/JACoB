#!/usr/bin/env bash
set -euo pipefail
mkdir -p dist
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -o dist/jacob-linux-amd64 ./cmd/jacob
