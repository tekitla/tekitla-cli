# tekitla-cli: CLI tk de Tekitla.
#
#   make build   binario en bin/tk
#   make test    tests (sin red: servidor HTTP falso)
#   make lint    gofmt + go vet + golangci-lint
#   make vuln    govulncheck
#
# Versiones fijas de las herramientas: las mismas que usa el CI.

GOLANGCI_VERSION ?= v2.15.0
GOVULNCHECK_VERSION ?= v1.1.4

BIN := bin/tk
LDFLAGS := -s -w -X github.com/tekitla/tekitla-cli/internal/cli.Version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: help build test lint vuln fmt tidy clean

## help: lista los targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'

## build: compila bin/tk
build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/tk

## test: tests
test:
	go test ./... -count=1

## lint: gofmt, go vet y golangci-lint (instalado, o con go run en la versión fija)
lint:
	@out="$$(gofmt -l cmd internal)"; if [ -n "$$out" ]; then echo "gofmt pendiente en:"; echo "$$out"; exit 1; fi
	go vet ./...
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run ./...; \
	else go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) run ./...; fi

## vuln: govulncheck sobre el código y sus dependencias
vuln:
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

## fmt: gofmt -w
fmt:
	gofmt -w cmd internal

## tidy: go mod tidy
tidy:
	go mod tidy

## clean: borra bin/ y dist/
clean:
	rm -rf bin dist
