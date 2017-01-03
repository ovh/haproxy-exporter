AGENTS=api scheduler aggregator worker
BUILD_DIR=build

CC=go build
GITHASH=$(shell git rev-parse HEAD)
CFLAGS=-race -ldflags "-X github.com/runabove/haproxy-exporter/cmd.githash=$(GITHASH)"

rwildcard=$(foreach d,$(wildcard $1*),$(call rwildcard,$d/,$2) $(filter $(subst *,%,$2),$d))
VPATH= $(BUILD_DIR)

.SECONDEXPANSION:

build: haproxy-exporter.go $$(call rwildcard, ./cmd, *.go) $$(call rwildcard, ./core, *.go)
	$(CC) $(CFLAGS) -o $(BUILD_DIR)/haproxy-exporter haproxy-exporter.go

.PHONY: lint
lint:
	@command -v gometalinter >/dev/null 2>&1 || { echo >&2 "gometalinter is required but not available please follow instructions from https://github.com/alecthomas/gometalinter"; exit 1; }
	gometalinter --deadline=180s --disable-all --enable=gofmt ./cmd/... ./core/... ./
	gometalinter --deadline=180s --disable-all --enable=vet ./cmd/... ./core/... ./
	gometalinter --deadline=180s --disable-all --enable=golint ./cmd/... ./core/... ./
	gometalinter --deadline=180s --disable-all --enable=ineffassign ./cmd/... ./core/... ./
	gometalinter --deadline=180s --disable-all --enable=misspell ./cmd/... ./core/... ./
	gometalinter --deadline=180s --disable-all --enable=staticcheck ./cmd/... ./core/... ./

.PHONY: format
format:
	gofmt -w -s ./cmd ./core haproxy-exporter.go

.PHONY: dev
dev: format lint build

.PHONY: clean
clean:
	-rm -r build
