# Acksy developer tasks. Run `make help` for the list.

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG     := github.com/oowliq/acksy/internal/buildinfo
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)

BIN     := bin/acksy
PNPM    := pnpm -C web
COMPOSE := docker compose -f compose.dev.yaml

# Integration test dependencies (see compose.dev.yaml).
export ACKSY_TEST_POSTGRES_URL ?= postgres://acksy:acksy@localhost:55432/acksy?sslmode=disable
export ACKSY_TEST_REDIS_URL    ?= redis://localhost:56379/0

.DEFAULT_GOAL := help
.PHONY: help deps dev dev-api dev-web web build test test-go test-web test-integration \
        lint lint-go lint-web fmt generate deps-up deps-down docker demo clean

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

deps: ## Install Go modules and web dependencies
	go mod download
	$(PNPM) install

dev: ## Run API (:8080) and Vite dev server (:5173) together
	@$(MAKE) -j2 dev-api dev-web

dev-api: ## Run the Go server with debug logging
	ACKSY_LOG_LEVEL=debug go run ./cmd/acksy serve

dev-web: ## Run the Vite dev server (proxies /api to :8080)
	$(PNPM) dev

web: ## Build the SPA into web/dist
	$(PNPM) build

build: web ## Build the single binary with the embedded SPA
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/acksy

test: test-go test-web ## Unit tests (Go on SQLite + web)

test-go: ## Go unit tests (Postgres/Redis tests skip without URLs)
	ACKSY_TEST_POSTGRES_URL= ACKSY_TEST_REDIS_URL= go test -race ./...

test-web: ## Web unit tests
	$(PNPM) test

test-integration: deps-up ## Go tests against Postgres and Redis from compose.dev.yaml
	go test -race -count=1 ./...

lint: lint-go lint-web ## All linters

lint-go: ## golangci-lint (install: https://golangci-lint.run/docs/welcome/install/)
	golangci-lint run

lint-web: ## Biome (lint + format check) + TypeScript
	$(PNPM) lint
	$(PNPM) typecheck

fmt: ## Format Go (gofumpt, goimports) and web (Biome)
	golangci-lint fmt
	$(PNPM) format

generate: ## Regenerate TypeScript API types from api/openapi.yaml (Kubb)
	$(PNPM) gen:api

deps-up: ## Start Postgres and Redis for integration tests
	$(COMPOSE) up -d --wait

deps-down: ## Stop integration test dependencies
	$(COMPOSE) down -v

docker: ## Build the Docker image
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg DATE=$(DATE) -t acksy:$(VERSION) .

demo: ## Fill a running local server with synthetic demo notifications
	./scripts/demo-seed.sh

clean: ## Remove build outputs
	rm -rf bin web/dist/assets web/dist/index.html web/dist/favicon.svg
