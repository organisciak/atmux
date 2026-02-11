BINARY_NAME=atmux
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X github.com/porganisciak/agent-tmux/cmd.Version=$(VERSION) -X github.com/porganisciak/agent-tmux/cmd.Commit=$(COMMIT) -X github.com/porganisciak/agent-tmux/cmd.BuildDate=$(BUILD_DATE)"

.PHONY: build test install clean release

build:
	go build $(LDFLAGS) -o $(BINARY_NAME) .

test:
	go test ./...

install: build
	cp $(BINARY_NAME) "$$(brew --prefix)/bin/"
	codesign -f -s - "$$(brew --prefix)/bin/$(BINARY_NAME)"
	@echo "Installed to $$(brew --prefix)/bin/$(BINARY_NAME)"

clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/

release:
	mkdir -p dist
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-arm64 .
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-arm64 .
