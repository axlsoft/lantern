.PHONY: bootstrap lint test proto-lint ui-install ui-build

## Install all tools and dependencies, start compose services.
bootstrap:
	@echo "→ Checking required tools..."
	@command -v go      >/dev/null || (echo "ERROR: go not found"; exit 1)
	@command -v docker  >/dev/null || (echo "ERROR: docker not found"; exit 1)
	@command -v pnpm    >/dev/null || (echo "ERROR: pnpm not found (npm install -g pnpm)"; exit 1)
	@command -v buf     >/dev/null || (echo "ERROR: buf not found (brew install bufbuild/buf/buf)"; exit 1)
	@echo "→ Installing Go tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	@echo "→ Installing UI dependencies..."
	pnpm install --dir ui
	@echo "→ Starting local services (Postgres + MailHog)..."
	docker compose -f deploy/docker/compose.dev.yml up -d
	@echo "✓ Bootstrap complete. Run 'make lint' and 'make test' to verify."

## Run all linters: Go (golangci-lint), UI (eslint + prettier + svelte-check), proto (buf).
lint:
	golangci-lint run ./...
	pnpm --dir ui lint
	pnpm --dir ui check
	buf lint proto

## Run all tests: Go unit tests + UI vitest.
test:
	go test ./...
	pnpm --dir ui test

## Install UI dependencies only (no docker required).
ui-install:
	pnpm install --dir ui

## Build the UI static bundle.
ui-build:
	pnpm --dir ui build

## Lint proto schemas only.
proto-lint:
	buf lint proto
