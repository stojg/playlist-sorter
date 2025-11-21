# ABOUTME: Simple build shortcuts for PGO optimization
# ABOUTME: Wraps go commands with project-specific flags

.PHONY: dev prod test clean fmt lint vuln check

dev:
	go build -race -o playlist-sorter-dev

prod: default.pgo
	go build -pgo=auto -ldflags="-s -w" -trimpath -o playlist-sorter
	go install -pgo=auto -ldflags="-s -w" -trimpath

default.pgo:
	go build -o playlist-sorter-pgo
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
	go tool golangci-lint run

vuln:
	go tool govulncheck ./...

check: fmt lint vuln
