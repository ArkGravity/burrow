.DEFAULT_GOAL := help

GO ?= go
BUN ?= bun
PRETTIER ?= prettier
CONFIG ?= configs/config.yaml
ADMIN_USERNAME ?= admin
COMPOSE_ENV ?= .env
COMPOSE_PROJECT ?= burrow
COMPOSE_PROFILES ?= local-db
COMPOSE = docker compose --env-file "$(COMPOSE_ENV)" --project-name "$(COMPOSE_PROJECT)" $(foreach profile,$(COMPOSE_PROFILES),--profile $(profile))

.PHONY: help deps run migrate admin-init keys-rotate build test test-db test-e2e lint fmt check web-install web web-dev web-check examples-install browser-install compose-config compose-build compose-up compose-down compose-logs compose-admin

help: ## Show available commands
	@awk 'BEGIN {FS = ":.*## "; printf "Burrow development commands\n\n"} /^[a-zA-Z_-]+:.*## / {printf "  %-19s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

deps: web-install ## Download backend and frontend dependencies
	$(GO) mod download

run: ## Run the backend using CONFIG (default: configs/config.yaml)
	$(GO) run ./cmd/burrow serve --config "$(CONFIG)"

migrate: ## Apply database migrations and initialize signing keys
	$(GO) run ./cmd/burrow migrate --config "$(CONFIG)"

admin-init: ## Create the initial administrator; read its password from stdin
	$(GO) run ./cmd/burrow admin-init --config "$(CONFIG)" --username "$(ADMIN_USERNAME)" --password-stdin

keys-rotate: ## Rotate OIDC signing keys while retaining existing public keys
	$(GO) run ./cmd/burrow keys-rotate --config "$(CONFIG)"

build: web ## Build bin/burrow with the frontend embedded
	@mkdir -p bin
	$(GO) build -tags embedweb -trimpath -o bin/burrow ./cmd/burrow

test: ## Run Go tests with race detection (PostgreSQL requires its test DSN)
	$(GO) test -race ./... -count=1

test-db: ## Require BURROW_TEST_POSTGRES_DSN and run the Go suite
	@test -n "$$BURROW_TEST_POSTGRES_DSN" || { echo "Set BURROW_TEST_POSTGRES_DSN to a dedicated test database"; exit 1; }
	$(GO) test -race ./... -count=1

test-e2e: examples-install ## Run Chromium tests and independent OIDC clients
	sh web/e2e/run.sh

lint: ## Run Go static analysis
	$(GO) vet ./...

fmt: ## Format Go and frontend sources using local gofmt and Prettier
	gofmt -w cmd configs internal examples/web-client web/*.go
	$(PRETTIER) --write web/src web/e2e web/*.ts examples/spa-client/main.ts

check: lint test web-check ## Run backend and frontend checks

web-install: ## Install locked frontend dependencies with Bun
	$(BUN) install --cwd web --frozen-lockfile

web: web-install ## Build the frontend
	$(BUN) run --cwd web build

web-dev: ## Run the Vite development server
	$(BUN) run --cwd web dev

web-check: ## Typecheck and test the frontend
	$(BUN) run --cwd web typecheck
	$(BUN) run --cwd web test

examples-install: ## Install the independent SPA client's locked dependencies
	$(BUN) install --cwd examples/spa-client --frozen-lockfile

browser-install: ## Install Chromium for Playwright tests
	cd web && $(BUN) x playwright install chromium

compose-config: ## Validate Compose configuration without printing secrets
	$(COMPOSE) config --quiet

compose-build: ## Build the container image
	$(COMPOSE) build

compose-up: ## Start the configured Compose stack using local images
	$(COMPOSE) up -d --no-build --pull never

compose-down: ## Stop the Compose stack while preserving database volumes
	$(COMPOSE) down

compose-logs: ## Follow application and migration logs
	$(COMPOSE) logs -f app migrate

compose-admin: ## Initialize the Compose administrator from stdin
	$(COMPOSE) run --rm --no-deps -T app admin-init --username "$(ADMIN_USERNAME)" --password-stdin
