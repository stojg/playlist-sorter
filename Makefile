# ABOUTME: Build automation for playlist-sorter with PGO and optimization
# ABOUTME: Handles profile collection, production builds, and installation

.PHONY: help dev profile-collect build-release install clean test lint all

# Default target
.DEFAULT_GOAL := help

# Configuration
BINARY_NAME := playlist-sorter
PROFILE_PLAYLIST := 100_random.m3u8
PROFILE_DURATION := 30
BUILD_FLAGS := -trimpath
LDFLAGS := -s -w
PGO_FLAGS := -pgo=auto

# Colors for output
GREEN := \033[0;32m
YELLOW := \033[0;33m
RED := \033[0;31m
NC := \033[0m # No Color

## help: Show this help message
help:
	@echo "$(GREEN)Playlist Sorter - Build Targets$(NC)"
	@echo ""
	@echo "$(YELLOW)Development:$(NC)"
	@echo "  make dev              Build development binary (with debug info, race detector)"
	@echo "  make test             Run tests"
	@echo "  make lint             Run linter"
	@echo ""
	@echo "$(YELLOW)Production:$(NC)"
	@echo "  make profile-collect  Collect PGO profile (30s run on $(PROFILE_PLAYLIST))"
	@echo "  make build-release    Build optimized production binary (PGO + stripped)"
	@echo "  make install          Build and install to GOPATH/bin"
	@echo "  make all              Profile + Build + Install (complete production setup)"
	@echo ""
	@echo "$(YELLOW)Maintenance:$(NC)"
	@echo "  make clean            Remove built binaries and profiles"
	@echo "  make clean-profile    Remove only PGO profile"
	@echo "  make stats            Show binary size comparison"
	@echo ""
	@echo "$(GREEN)Quick Start:$(NC)"
	@echo "  make all              # One command for full production setup"

## dev: Build development binary with race detector and debug info
dev:
	@echo "$(GREEN)[DEV]$(NC) Building development binary..."
	go build -race -o $(BINARY_NAME)-dev
	@echo "$(GREEN)✓$(NC) Built: $(BINARY_NAME)-dev ($(shell du -h $(BINARY_NAME)-dev | cut -f1))"

## profile-collect: Collect PGO profile by running on test playlist
profile-collect:
	@echo "$(GREEN)[PROFILE]$(NC) Collecting PGO profile..."
	@if [ ! -f "$(PROFILE_PLAYLIST)" ]; then \
		echo "$(RED)✗$(NC) Error: $(PROFILE_PLAYLIST) not found"; \
		exit 1; \
	fi
	@echo "$(YELLOW)→$(NC) Building collector binary..."
	@go build -o $(BINARY_NAME)-collector
	@echo "$(YELLOW)→$(NC) Running for $(PROFILE_DURATION) seconds on $(PROFILE_PLAYLIST)..."
	@timeout $(PROFILE_DURATION) ./$(BINARY_NAME)-collector -cpuprofile=default.pgo $(PROFILE_PLAYLIST) > /dev/null 2>&1 || true
	@if [ -f "default.pgo" ]; then \
		echo "$(GREEN)✓$(NC) Profile collected: default.pgo ($(shell du -h default.pgo | cut -f1))"; \
		rm -f $(BINARY_NAME)-collector; \
	else \
		echo "$(RED)✗$(NC) Failed to collect profile"; \
		rm -f $(BINARY_NAME)-collector; \
		exit 1; \
	fi

## build-release: Build production binary with PGO and stripping
build-release:
	@echo "$(GREEN)[RELEASE]$(NC) Building production binary..."
	@if [ ! -f "default.pgo" ]; then \
		echo "$(YELLOW)⚠$(NC)  No PGO profile found, collecting..."; \
		$(MAKE) profile-collect; \
	fi
	@echo "$(YELLOW)→$(NC) Compiling with PGO and optimizations..."
	go build $(BUILD_FLAGS) $(PGO_FLAGS) -ldflags="$(LDFLAGS)" -o $(BINARY_NAME)
	@echo "$(GREEN)✓$(NC) Built: $(BINARY_NAME) ($(shell du -h $(BINARY_NAME) | cut -f1))"
	@echo "$(GREEN)✓$(NC) Optimizations: PGO + Stripped + Trimpath"

## install: Build production binary and install to GOPATH/bin
install: build-release
	@echo "$(GREEN)[INSTALL]$(NC) Installing to GOPATH/bin..."
	go install $(BUILD_FLAGS) $(PGO_FLAGS) -ldflags="$(LDFLAGS)"
	@echo "$(GREEN)✓$(NC) Installed: $(shell go env GOPATH)/bin/$(BINARY_NAME)"
	@echo "$(GREEN)✓$(NC) You can now run: $(BINARY_NAME)"

## all: Complete production setup (profile + build + install)
all: profile-collect build-release install
	@echo ""
	@echo "$(GREEN)═══════════════════════════════════════════$(NC)"
	@echo "$(GREEN)✓ Production setup complete!$(NC)"
	@echo "$(GREEN)═══════════════════════════════════════════$(NC)"
	@$(MAKE) stats

## test: Run all tests
test:
	@echo "$(GREEN)[TEST]$(NC) Running tests..."
	go test -v -race ./...
	@echo "$(GREEN)✓$(NC) Tests passed"

## lint: Run linter
lint:
	@echo "$(GREEN)[LINT]$(NC) Running golangci-lint..."
	go tool golangci-lint run
	@echo "$(GREEN)✓$(NC) Lint passed"

## stats: Show binary size comparison
stats:
	@echo ""
	@echo "$(GREEN)Binary Size Comparison:$(NC)"
	@if [ -f "$(BINARY_NAME)-dev" ]; then \
		echo "  Development (with debug): $(shell du -h $(BINARY_NAME)-dev | cut -f1)"; \
	fi
	@if [ -f "$(BINARY_NAME)" ]; then \
		echo "  Production (PGO+stripped): $(shell du -h $(BINARY_NAME) | cut -f1)"; \
	fi
	@if [ -f "default.pgo" ]; then \
		echo ""; \
		echo "$(GREEN)PGO Profile:$(NC)"; \
		echo "  Size: $(shell du -h default.pgo | cut -f1)"; \
		echo "  Age: $(shell stat -f '%Sm' -t '%Y-%m-%d %H:%M:%S' default.pgo 2>/dev/null || stat -c '%y' default.pgo 2>/dev/null | cut -d' ' -f1,2)"; \
	fi

## clean: Remove all built binaries and profiles
clean:
	@echo "$(GREEN)[CLEAN]$(NC) Removing built artifacts..."
	rm -f $(BINARY_NAME) $(BINARY_NAME)-dev $(BINARY_NAME)-collector
	rm -f $(BINARY_NAME)-debug $(BINARY_NAME)-stripped $(BINARY_NAME)-pgo*
	rm -f $(BINARY_NAME)-nopgo $(BINARY_NAME)-upx
	rm -f default.pgo cpu.prof mem.prof *.prof
	@echo "$(GREEN)✓$(NC) Cleaned"

## clean-profile: Remove only PGO profile (forces re-collection on next build)
clean-profile:
	@echo "$(GREEN)[CLEAN]$(NC) Removing PGO profile..."
	rm -f default.pgo
	@echo "$(GREEN)✓$(NC) Profile removed (will be re-collected on next build)"

# Utility targets (not shown in help)

# Check if required tools are installed
.PHONY: check-deps
check-deps:
	@command -v go >/dev/null 2>&1 || { echo "$(RED)✗$(NC) go is not installed"; exit 1; }
	@command -v timeout >/dev/null 2>&1 || { echo "$(RED)✗$(NC) timeout is not installed"; exit 1; }

# Verify build
.PHONY: verify
verify: build-release
	@echo "$(GREEN)[VERIFY]$(NC) Testing built binary..."
	@./$(BINARY_NAME) --help > /dev/null 2>&1 || { echo "$(RED)✗$(NC) Binary doesn't run"; exit 1; }
	@echo "$(GREEN)✓$(NC) Binary works"
