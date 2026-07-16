.PHONY: build test integration fmt vet

VERSION ?= $(shell cat VERSION)
LDFLAGS := -s -w -X main.version=$(VERSION)

build:
	mkdir -p bin
	go build -trimpath -buildvcs=true -ldflags "$(LDFLAGS)" -o bin/gia ./cmd/gia

test:
	go test ./...

integration: build
	./scripts/test-local.sh

fmt:
	gofmt -w ./cmd ./internal

vet:
	go vet ./...
