BINARY=hermes
VERSION=0.7.0
BUILD_TIME=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

.PHONY: all build test clean install run lint

all: build

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/hermes/

install: build
	mkdir -p ~/.local/bin
	cp $(BINARY) ~/.local/bin/

run: build
	./$(BINARY)

test:
	go test ./... -v -count=1

test-short:
	go test ./... -short -count=1

test-race:
	go test ./... -race -count=1

lint:
	go vet ./...

clean:
	rm -f $(BINARY)
	go clean

deps:
	go mod tidy
	go mod download

fmt:
	go fmt ./...

# Cross-compilation
build-linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY)-linux-amd64 ./cmd/hermes/
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY)-linux-arm64 ./cmd/hermes/

build-darwin:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY)-darwin-amd64 ./cmd/hermes/
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY)-darwin-arm64 ./cmd/hermes/

build-all: build-linux build-darwin
