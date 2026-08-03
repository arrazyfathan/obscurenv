SHELL := /bin/sh

BIN_DIR ?= bin
DIST_DIR ?= dist
INSTALL_DIR ?= $(HOME)/.local/bin
BACKEND_ADDR ?= :8080
DATABASE_URL ?= postgres://obv:obv@localhost:5432/obv?sslmode=disable
GOCACHE ?= $(CURDIR)/.gocache
GOMODCACHE ?= $(CURDIR)/.gomodcache
VERSION ?= $(shell cat VERSION)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || printf 'unknown')
BUILT_AT ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
LDFLAGS := -X github.com/obscurenv/obscurenv/cli/cmd.version=$(VERSION) -X github.com/obscurenv/obscurenv/cli/cmd.commit=$(COMMIT) -X github.com/obscurenv/obscurenv/cli/cmd.builtAt=$(BUILT_AT)

export GOCACHE
export GOMODCACHE

.PHONY: help version check-version build install package release test vet check backend up down clean

help:
	@printf '%s\n' 'Available targets:'
	@printf '%s\n' '  make version    Print the CLI version configured for builds'
	@printf '%s\n' '  make build      Build the obe CLI into ./bin/obe'
	@printf '%s\n' '  make install    Build and install obe using ./install.sh'
	@printf '%s\n' '  make package    Build and package obe into ./dist'
	@printf '%s\n' '  make release    Run checks, then package obe into ./dist'
	@printf '%s\n' '  make test       Run Go tests for backend and CLI'
	@printf '%s\n' '  make vet        Run go vet for backend and CLI'
	@printf '%s\n' '  make check      Run test and vet'
	@printf '%s\n' '  make backend    Run the backend locally'
	@printf '%s\n' '  make up         Start backend and PostgreSQL with Docker Compose'
	@printf '%s\n' '  make down       Stop Docker Compose services'
	@printf '%s\n' '  make clean      Remove local build output'

version:
	@printf '%s\n' '$(VERSION)'

check-version:
	@printf '%s\n' '$(VERSION)' | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$$' || { printf '%s\n' 'VERSION must be valid SemVer, for example 0.1.0 or 0.1.0-beta.1' >&2; exit 1; }

build: check-version
	@mkdir -p $(BIN_DIR)
	cd cli && GOOS="$(GOOS)" GOARCH="$(GOARCH)" go build -ldflags "$(LDFLAGS)" -o ../$(BIN_DIR)/obe .

install: check-version
	VERSION="$(VERSION)" COMMIT="$(COMMIT)" BUILT_AT="$(BUILT_AT)" INSTALL_DIR="$(INSTALL_DIR)" ./install.sh

package: build
	@mkdir -p $(DIST_DIR)
	tar -C $(BIN_DIR) -czf $(DIST_DIR)/obe_$(VERSION)_$(GOOS)_$(GOARCH).tar.gz obe

release: check package

test:
	go test ./backend/... ./cli/...

vet:
	go vet ./backend/... ./cli/...

check: test vet

backend:
	cd backend && DATABASE_URL="$(DATABASE_URL)" ADDR="$(BACKEND_ADDR)" go run .

up:
	docker compose up -d

down:
	docker compose down

clean:
	rm -rf $(BIN_DIR)
