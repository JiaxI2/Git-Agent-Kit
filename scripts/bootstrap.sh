#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/.."
mkdir -p bin
go test ./...
go build -o bin/gia ./cmd/gia
printf 'Built: %s/bin/gia\n' "$(pwd)"
