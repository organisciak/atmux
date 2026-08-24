BINARY_NAME=atmux
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X github.com/porganisciak/agent-tmux/cmd.Version=$(VERSION) -X github.com/porganisciak/agent-tmux/cmd.Commit=$(COMMIT) -X github.com/porganisciak/agent-tmux/cmd.BuildDate=$(BUILD_DATE)"

.PHONY: build test install clean release verify-release tag-version brew-bump version-status install-hooks

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

# Session history is SQLite via go-sqlite3, which needs cgo. Cross-compiling
# turns cgo off by default, and go-sqlite3 then silently links a stub that
# fails at runtime with "Binary was compiled with 'CGO_ENABLED=0'". Every
# cross-built binary must therefore set CGO_ENABLED=1 and a matching CC.
#
# macOS ships a universal SDK, so a mac can build both darwin arches. Linux
# targets need a real cross toolchain; set CC_LINUX_AMD64 / CC_LINUX_ARM64
# (e.g. to a zig cc wrapper) to build them. Without one they are skipped
# rather than shipped broken.
release:
	rm -rf dist
	mkdir -p dist
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 CC="clang -arch x86_64" \
		go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-amd64 .
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 CC="clang -arch arm64" \
		go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-arm64 .
	@if [ -n "$(CC_LINUX_AMD64)" ]; then \
		CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC="$(CC_LINUX_AMD64)" \
			go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-amd64 . ; \
	else \
		echo "SKIP linux-amd64: set CC_LINUX_AMD64 to a cgo cross-compiler (history needs cgo)" ; \
	fi
	@if [ -n "$(CC_LINUX_ARM64)" ]; then \
		CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC="$(CC_LINUX_ARM64)" \
			go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-arm64 . ; \
	else \
		echo "SKIP linux-arm64: set CC_LINUX_ARM64 to a cgo cross-compiler (history needs cgo)" ; \
	fi
	@$(MAKE) --no-print-directory verify-release

# verify-release fails the build if any produced binary lost cgo, which is the
# failure mode that shipped a broken history database to a remote host.
verify-release:
	@for f in dist/$(BINARY_NAME)-*; do \
		if go version -m "$$f" 2>/dev/null | grep -q "CGO_ENABLED=0"; then \
			echo "ERROR: $$f was built without cgo; session history would be a stub" ; \
			exit 1 ; \
		fi ; \
	done
	@echo "All release binaries have cgo enabled."

tag-version:
	./scripts/tag-version.sh "$(VERSION)" $(if $(PUSH),--push,)

brew-bump:
	./scripts/brew-bump.sh "$(VERSION)"

version-status:
	./scripts/commits-since-version.sh

install-hooks:
	git config core.hooksPath .githooks
	@echo "Installed hooks path: .githooks"
