# ABOUTME: Simple build shortcuts for PGO optimization
# ABOUTME: Wraps go commands with project-specific flags

.PHONY: dev install test clean fmt lint vuln check

dev:
	go build -race -o playlist-sorter-dev

install: default.pgo
	go build -pgo=auto -ldflags="-s -w" -trimpath -o playlist-sorter
	go install -pgo=auto -ldflags="-s -w" -trimpath

default.pgo:
	go build -o playlist-sorter-pgo
	shuf ./100_random.m3u8 -o 100_random.m3u8
	timeout 30 ./playlist-sorter-pgo -cpuprofile=default.pgo 100_random.m3u8 || true
	rm -f playlist-sorter-pgo

test:
	go test -v -race ./...

clean:
	rm -f playlist-sorter playlist-sorter-dev playlist-sorter-pgo default.pgo *.prof

fmt:
	go tool gofumpt -l -w .
	go tool goimports -w .

lint:
	@echo "Running go vet..."
	-go vet ./...
	@echo "Running errcheck..."
	-go tool errcheck ./...
	@echo "Running staticcheck (includes gosimple, unused)..."
	-go tool staticcheck ./...
	@echo "Running ineffassign..."
	-go tool ineffassign ./...
	@echo "Running revive..."
	-go tool revive -formatter friendly ./...
	@echo "Running exhaustive..."
	-go tool exhaustive ./...
	@echo "Running deadcode..."
	-go tool deadcode ./...

vuln:
	go tool govulncheck ./...

check: fmt lint vuln
