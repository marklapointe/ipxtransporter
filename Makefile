# SPDX-License-Identifier: BSD-3-Clause
# IPXTransporter Makefile

BINARY_NAME=ipxtransporter
DIST_DIR=dist
VERSION=1.0.0

# Platform detection
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
    OS := Linux
    # Check if it's Ubuntu/Debian
    DISTRO := $(shell lsb_release -is 2>/dev/null || cat /etc/os-release | grep ^ID= | cut -d= -f2 | tr -d '"' | tr '[:upper:]' '[:lower:]')
else ifeq ($(UNAME_S),FreeBSD)
    OS := FreeBSD
endif

.PHONY: help all build clean test deb rpm run run-daemon run-demo demo fmt vet install-deps man install

all: help

help:
	@echo "IPXTransporter Makefile"
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  help           - Show this help message"
	@echo "  build          - Build the binary ($(BINARY_NAME))"
	@echo "  install        - Install the binary and default configuration"
	@echo "  install-deps   - Install system dependencies (libpcap)"
	@echo "  test           - Run unit tests"
	@echo "  run            - Build and run in TUI mode (debug-only SSL)"
	@echo "  run-daemon     - Build and run in daemon mode (debug-only SSL)"
	@echo "  run-demo       - Build and run in TUI mode with demo data"
	@echo "  demo           - Shortcut for run-demo"
	@echo "  fmt            - Format Go source code"
	@echo "  vet            - Run static analysis"
	@echo "  man            - View the man page"
	@echo "  clean          - Remove binary and distribution files"
	@echo "  deb            - Build Debian package"
	@echo "  rpm            - Build RPM package"

install-deps:
	@echo "OS: $(OS), DISTRO: $(DISTRO)"
ifeq ($(OS),Linux)
ifeq ($(shell echo $(DISTRO) | tr '[:upper:]' '[:lower:]'),ubuntu)
	sudo apt-get update && sudo apt-get install -y libpcap-dev build-essential
else ifeq ($(shell echo $(DISTRO) | tr '[:upper:]' '[:lower:]'),debian)
	sudo apt-get update && sudo apt-get install -y libpcap-dev build-essential
else
	@echo "Unsupported Linux distribution: $(DISTRO). Please install libpcap-dev manually."
endif
else ifeq ($(OS),FreeBSD)
	sudo pkg install -y libpcap
else
	@echo "Unsupported platform: $(UNAME_S). Please install libpcap manually."
endif

build:
	go build -o $(BINARY_NAME) ./cmd/ipxtransporter

run: build
	./$(BINARY_NAME) --disable-ssl --tui=true

run-daemon: build
	./$(BINARY_NAME) --disable-ssl --tui=false

run-demo: build
	./$(BINARY_NAME) --demo --tui=true

demo: run-demo

test:
	go test ./internal/config ./internal/stats ./internal/logger ./internal/peer ./internal/relay

fmt:
	go fmt ./...

vet:
	go vet ./...

man:
	man ./ipxtransporter.8

install: build
ifeq ($(OS),Linux)
	install -m 755 $(BINARY_NAME) /usr/local/bin/
	[ -f /etc/ipxtransporter.json ] || install -m 644 examples/ipxtransporter.json.example /etc/ipxtransporter.json
else ifeq ($(OS),FreeBSD)
	install -m 755 $(BINARY_NAME) /usr/local/bin/
	[ -f /usr/local/etc/ipxtransporter.json ] || install -m 644 examples/ipxtransporter.json.example /usr/local/etc/ipxtransporter.json
endif

clean:
	rm -f $(BINARY_NAME)
	rm -rf $(DIST_DIR)

# Simplified packaging targets for demonstration.
# In a real environment, these would use fpm or native tools.
deb: build
	mkdir -p $(DIST_DIR)
	@echo "Creating Debian package (stub)..."
	touch $(DIST_DIR)/$(BINARY_NAME)_$(VERSION)_amd64.deb

rpm: build
	mkdir -p $(DIST_DIR)
	@echo "Creating RPM package (stub)..."
	touch $(DIST_DIR)/$(BINARY_NAME)-$(VERSION)-1.x86_64.rpm
