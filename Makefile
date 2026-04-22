.PHONY: test test-race test-cover fuzz-quick fuzz-long build install tidy release

# Standard test run (all packages, no race detector).
test:
	go test ./...

# Race-detector run. Slower (~20s) but catches concurrency bugs that
# plain `go test` misses — always run before shipping a release.
test-race:
	go test ./... -race

# Print per-package coverage.
test-cover:
	go test ./... -cover

# Short fuzz pass (8s per target). Use for PR gating / local sanity.
fuzz-quick:
	go test ./internal/detector/ -run=^$$ -fuzz=^FuzzClaude$$      -fuzztime=8s
	go test ./internal/detector/ -run=^$$ -fuzz=^FuzzCodex$$       -fuzztime=8s
	go test ./internal/detector/ -run=^$$ -fuzz=^FuzzCursor$$      -fuzztime=8s
	go test ./internal/detector/ -run=^$$ -fuzz=^FuzzFuzzyMatch$$  -fuzztime=8s
	go test ./internal/screen/   -run=^$$ -fuzz=^FuzzScreenFeed$$  -fuzztime=8s

# Long fuzz pass (2m per target). Use for periodic robustness runs.
fuzz-long:
	go test ./internal/detector/ -run=^$$ -fuzz=^FuzzClaude$$      -fuzztime=2m
	go test ./internal/detector/ -run=^$$ -fuzz=^FuzzCodex$$       -fuzztime=2m
	go test ./internal/detector/ -run=^$$ -fuzz=^FuzzCursor$$      -fuzztime=2m
	go test ./internal/detector/ -run=^$$ -fuzz=^FuzzFuzzyMatch$$  -fuzztime=2m
	go test ./internal/screen/   -run=^$$ -fuzz=^FuzzScreenFeed$$  -fuzztime=2m

build:
	go build -o yoyo ./cmd/yoyo

install:
	go install ./cmd/yoyo

tidy:
	go mod tidy

# Manual release: build 4 platforms, generate checksums, upload to GitHub
# Release with install.sh. TAG must be set (e.g. make release TAG=v2.3.0).
# The tag must already exist locally and on origin.
release:
ifndef TAG
	$(error TAG is not set — usage: make release TAG=v2.3.0)
endif
	./scripts/release.sh $(TAG)
