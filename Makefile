BINARY_NAME=atmux
BREW_FORMULA=atmux
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X github.com/porganisciak/agent-tmux/cmd.Version=$(VERSION) -X github.com/porganisciak/agent-tmux/cmd.Commit=$(COMMIT) -X github.com/porganisciak/agent-tmux/cmd.BuildDate=$(BUILD_DATE)"

.PHONY: default brew-update build install clean test release

default: brew-update

brew-update:
	brew update
	brew reinstall $(BREW_FORMULA) || brew install $(BREW_FORMULA)

build:
	go build $(LDFLAGS) -o $(BINARY_NAME) .

install: build
	cp $(BINARY_NAME) /usr/local/bin/
	@echo "Installed to /usr/local/bin/$(BINARY_NAME)"

install-brew: build
	cp $(BINARY_NAME) /opt/homebrew/bin/
	@echo "Installed to /opt/homebrew/bin/$(BINARY_NAME)"

install-home: build
	@# Avoid conflict if source dir is ~/bin/atmux
	@if [ -d ~/bin/$(BINARY_NAME) ]; then \
		cp $(BINARY_NAME) ~/bin/$(BINARY_NAME)/$(BINARY_NAME).bin && \
		ln -sf ~/bin/$(BINARY_NAME)/$(BINARY_NAME).bin ~/bin/$(BINARY_NAME)-cli && \
		echo "Installed to ~/bin/$(BINARY_NAME)-cli (symlink to avoid dir conflict)"; \
	else \
		cp $(BINARY_NAME) ~/bin/; \
		echo "Installed to ~/bin/$(BINARY_NAME)"; \
	fi

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
