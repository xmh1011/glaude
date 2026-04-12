VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BINARY  := glaude
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test lint cross clean

## build: compile the binary for the current platform
build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/glaude

## test: run all tests with race detector and coverage
test:
	go test -race -cover ./...

## lint: run go vet
lint:
	go vet ./...

## cross: build for all target platforms into dist/
cross: clean
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-amd64   ./cmd/glaude
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-arm64   ./cmd/glaude
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-amd64  ./cmd/glaude
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-arm64  ./cmd/glaude
	@echo "Built 4 binaries in dist/"

## clean: remove build artifacts
clean:
	rm -rf dist/ $(BINARY)
