.PHONY: build test integration fmt vet package
build:
	mkdir -p bin
	go build -o bin/gia ./cmd/gia

test:
	go test ./...

integration: build
	./scripts/test-local.sh

fmt:
	gofmt -w ./cmd ./internal

vet:
	go vet ./...

package: test vet build
	zip -r git-isolated-agent-kit.zip . -x '.git/*' 'git-isolated-agent-kit.zip'
