# Lantern

Lantern is an open-source functional test coverage platform. It instruments your test suites, collects coverage events via OTLP, and surfaces gaps in your functional test coverage — mapping tests to the code paths they exercise.

This repository is the **monorepo** for the Lantern collector (Go) and web UI (SvelteKit). SDK repos live separately:

- [lantern-dotnet](https://github.com/axlsoft/lantern-dotnet) — .NET SDK
- [lantern-node](https://github.com/axlsoft/lantern-node) — Node.js SDK
- [lantern-playwright](https://github.com/axlsoft/lantern-playwright) — Playwright plugin

## License

[AGPL-3.0](LICENSE) — collector and UI are AGPL-3.0. SDKs are Apache 2.0.

## Quickstart for contributors

```bash
make bootstrap
```

Prerequisites: Go 1.24+, Docker, pnpm, buf. The bootstrap command installs Go tools, UI dependencies, and starts Postgres + MailHog via Docker Compose.

After bootstrap:

```bash
make lint    # golangci-lint + UI lint/check + buf lint
make test    # go test ./... + UI vitest
```

## Repository Layout

```
cmd/collector/   Go main for the collector binary
internal/        Go internal packages (api, auth, db, ingest, github, tenancy)
pkg/             Go packages for external import (minimal)
ui/              SvelteKit + Svelte 5 web UI (Tailwind, shadcn-svelte)
proto/           Protobuf schemas shared by collector and SDKs
migrations/      SQL migration files
deploy/docker/   Dockerfile + Compose files
docs/adr/        Architecture Decision Records
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md), and [SECURITY.md](SECURITY.md).

See [CONTRIBUTING.md](CONTRIBUTING.md), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md), and [SECURITY.md](SECURITY.md).
