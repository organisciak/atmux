BINARY_NAME=agent-tmux
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X github.com/porganisciak/agent-tmux/cmd.Version=$(VERSION) -X github.com/porganisciak/agent-tmux/cmd.Commit=$(COMMIT) -X github.com/porganisciak/agent-tmux/cmd.BuildDate=$(BUILD_DATE)"

.PHONY: build install clean test release

build:
	go build $(LDFLAGS) -o $(BINARY_NAME) .

install: build
	cp $(BINARY_NAME) /usr/local/bin/
	@echo "Installed to /usr/local/bin/$(BINARY_NAME)"

install-home: build
	cp $(BINARY_NAME) ~/bin/
	@echo "Installed to ~/bin/$(BINARY_NAME)"

clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/

test:
	go test ./...

# Build for multiple platforms
release:
	mkdir -p dist
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-arm64 .
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-arm64 .
