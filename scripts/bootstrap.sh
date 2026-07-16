#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/.."
mkdir -p bin
VERSION=$(cat VERSION)
go test ./...
go build -trimpath -buildvcs=true -ldflags "-s -w -X main.version=$VERSION" -o bin/gia ./cmd/gia
printf 'Built: %s/bin/gia\n' "$(pwd)"
