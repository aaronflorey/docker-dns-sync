<!-- GSD:project-start source:PROJECT.md -->
## Project

**docker-dns-sync**

`docker-dns-sync` is a Go daemon for self-hosting operators who run Godoxy-labeled Docker workloads behind AdGuard Home. It watches Docker, derives DNS rewrites from Godoxy-compatible labels, and synchronizes only daemon-owned rewrites into AdGuard Home so local DNS routing stays current without manual cleanup.

The system is intentionally built around reconciliation and local ownership tracking so it can recover safely after daemon restarts, Docker event disconnects, or host reboots without touching operator-managed AdGuard records.

**Core Value:** Operators get correct local DNS rewrites for eligible Docker workloads automatically, quickly, and safely without breaking manual AdGuard records.

### Constraints

- **Tech stack**: Go daemon using the Docker Go SDK, AdGuard Home HTTP API, TOML config, and a persisted local state file — required by the product architecture.
- **Performance**: Create, update, and delete managed rewrites within 5 seconds of normal Docker lifecycle events — this is a primary success criterion.
- **Safety**: Never modify AdGuard rewrites that are not represented in daemon state — coexistence with manual records is mandatory.
- **Recovery**: Restart, reboot, and event-stream failure recovery must converge back to correct managed state — correctness cannot rely on a perfect live stream.
- **Security**: AdGuard credentials must come from config references or environment-backed secrets and must never be logged — secrets exposure is unacceptable.
- **Operations**: MVP must support both host-binary and Docker deployment — both are first-class operator environments.
<!-- GSD:project-end -->

<!-- GSD:stack-start source:research/STACK.md -->
## Technology Stack

## Recommended Stack
### Core Framework
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Go toolchain | 1.26.3 | Primary language/runtime | Current stable Go release as of research date. Strong fit for a long-running infra daemon: static binaries, cheap concurrency, strong stdlib, and first-class cross-compilation. Pin `go 1.26` in `go.mod` and `toolchain go1.26.3` for reproducible builds. | HIGH |
| Go standard library: `context`, `net/http`, `encoding/json`, `log/slog`, `flag`, `os/signal`, `sync`, `time` | Go 1.26.3 | Runtime primitives, HTTP client, state encoding, logging, process lifecycle | MVP does not need framework magic. The stdlib already covers cancellation, structured logging, HTTP, JSON, signals, and timers cleanly. This keeps the daemon small and easy to audit. | HIGH |
| In-process plugin contracts (plain Go interfaces) | Go 1.26.3 | Source/output extensibility | The PRD calls for plugin-oriented architecture, not separately deployed plugins. Plain interfaces keep future expansion possible without RPC, subprocesses, or version-skew problems. | HIGH |
### Database / State
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Local JSON state file using `encoding/json` + atomic temp-file write + rename | Go 1.26.3 | Persist daemon-owned rewrite state and ownership boundaries | State is small, single-writer, and deterministic. A JSON file is transparent for operators, easy to back up, and enough for restart/recovery semantics. Write to a temp file, `fsync`, then rename; use `0600` permissions where supported. | HIGH |
| No embedded database in MVP | — | Reduce moving parts | SQLite/Bolt/Badger add migration and corruption-recovery complexity without solving a demonstrated problem. This daemon needs durable ownership tracking, not ad hoc querying. | HIGH |
### Integrations / Infrastructure
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Docker Engine API v1.54 via `github.com/moby/moby/client` | API v1.54, module `v0.4.1` | Watch Docker events, inspect/list containers, derive desired state | Use the maintained Moby client instead of raw socket HTTP. Official Docker docs use the Moby client package and note API-version negotiation support, which matters for operator environments with mixed daemon versions. | HIGH |
| AdGuard Home Control API via handwritten `net/http` client | AdGuard Home API current through `v0.107.72` docs; rewrite endpoints added/updated through `v0.107.68` | List/add/delete/update rewrites and inspect rewrite settings | The rewrite API surface is small: `/control/rewrite/list`, `/add`, `/delete`, `/update`, and `/settings`. A small typed client is safer than pulling in a generated SDK for one narrow integration. Keep auth, JSON bodies, retries, and CSRF/content-type quirks explicit. | HIGH |
| Docker socket access via local Unix socket or socket proxy | Current Docker deployment guidance | Runtime connectivity | Matches PRD requirements and Docker’s security guidance. Support both `unix:///var/run/docker.sock` and proxy/TLS/SSH patterns through config; prefer least-privilege proxy setups when containerized. | HIGH |
| Host deployment: `systemd` unit | Current distro-managed runtime | First-class host-binary mode | This is the standard Linux daemon path for restart policy, environment files, file ownership, and startup ordering. It matches the operator persona better than inventing a custom supervisor story. | MEDIUM |
| Container deployment: minimal non-root OCI image (pin digest; prefer distroless static with CA certs) | Pin by digest at implementation time | First-class Docker mode | A minimal image keeps the attack surface small. Prefer a non-root image with CA certificates available so HTTPS to AdGuard Home works without extra packaging. | MEDIUM |
### Supporting Libraries
| Library | Version | Purpose | When to Use | Confidence |
|---------|---------|---------|-------------|------------|
| `github.com/BurntSushi/toml` | v1.6.0 | TOML config decoding/encoding | Use for explicit config structs and metadata-driven validation. It is stable, boring, TOML 1.1.0-compatible, and a better fit than config frameworks with hidden precedence rules. | HIGH |
| `github.com/cenkalti/backoff/v5` | v5.0.3 | Retry/backoff policy | Use if retry loops become non-trivial across AdGuard outages and Docker watch reconnects. Keep usage narrow and context-aware; do not wrap the whole reconciler in unbounded retries. | MEDIUM |
| `github.com/testcontainers/testcontainers-go` | v0.42.0 | Integration tests against real Docker and AdGuard Home | Use for startup reconcile, restart recovery, outage recovery, and end-to-end CRUD tests. It is the current standard Go container-test harness and is more future-facing than older Docker test helpers. | HIGH |
### Delivery / QA Tooling
| Tool | Version | Purpose | Why | Confidence |
|------|---------|---------|-----|------------|
| GoReleaser | v2.15.4 | Release host binaries and OCI images | Best fit for “host binary + Docker image” distribution from one config. It is the de facto Go release pipeline tool and avoids bespoke release scripting. | MEDIUM |
| golangci-lint | v2.12.2 | Lint aggregation in CI/local | Still the standard Go lint runner for catching error-handling, simplification, and style regressions without hand-curating many separate commands. Keep the ruleset modest. | MEDIUM |
| `go test`, fuzzing, and race detector | Go 1.26.3 | Unit and concurrency verification | The hardest bugs here are reconciliation edge cases, event races, and restart recovery. Lean on native Go test tooling first; add containerized integration tests where mocks stop being trustworthy. | HIGH |
## Alternatives Considered
| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| Config parsing | `BurntSushi/toml` | `spf13/viper` | Viper is powerful but too implicit for an infra daemon that must be predictable. Its multi-source precedence model is useful for apps with many input channels, but it adds avoidable config ambiguity here. |
| Config parsing | `BurntSushi/toml` | `github.com/pelletier/go-toml/v2` | `go-toml/v2` is viable, but BurntSushi is the more conservative choice for explicit struct decoding and simple operator-facing config. Either works; BurntSushi is the more boring MVP pick. |
| Docker integration | `github.com/moby/moby/client` | Raw HTTP over `/var/run/docker.sock` | Raw HTTP would duplicate auth, version negotiation, event streaming, and transport details the official client already handles. |
| Docker integration | `github.com/moby/moby/client` | Older `github.com/docker/docker/...` imports from stale examples | Current official docs and package ownership are under Moby. Avoid baking in older import paths from outdated blog posts. |
| AdGuard integration | Handwritten `net/http` client | Generated OpenAPI client | The API surface needed for MVP is tiny. Generated clients add regeneration churn, larger dependency trees, and less explicit control over auth/retry behavior. |
| State persistence | JSON file | SQLite (`modernc.org/sqlite`) | SQLite is excellent, but it is unnecessary for a small single-process ownership ledger. It adds schema and migration burden before the product needs query power. |
| Plugin architecture | In-process interfaces | `hashicorp/go-plugin` | Subprocess plugins solve a problem this project does not have yet. They complicate lifecycle, packaging, error handling, and cross-version compatibility. |
| Logging | `log/slog` | `zap` / `zerolog` | Both are good libraries, but MVP log volume is not high enough to justify extra dependencies. `slog` is good enough and standard. |
| CLI surface | stdlib `flag` | Cobra | This daemon likely needs only a small command surface (`run`, `validate-config`, maybe `inspect`). Cobra is unnecessary until the CLI becomes meaningfully broader. |
| Integration tests | `testcontainers-go` | `ory/dockertest` | `dockertest` still works, but Testcontainers has the stronger current ecosystem and a clearer path for richer dependency orchestration. |
| Observability | structured logs first | OpenTelemetry in MVP | OTel is useful later, but the PRD does not yet justify tracing/metrics complexity. Ship high-quality structured logs first; add metrics only when operational need is proven. |
## Implementation Notes
- **Config shape:** one root config struct with explicit nested `sources`, `outputs`, `state`, `logging`, and `retry` sections.
- **Validation:** parse TOML into structs, then run explicit semantic validation; reject unknown critical fields where practical.
- **State writes:** avoid partial writes; persist only after successful apply steps so local ownership remains trustworthy.
- **Docker watch model:** use event hints to trigger targeted re-listing or scoped reconciliation; do not treat the event stream as the source of truth.
- **AdGuard auth:** keep credentials/env references out of logs; always send JSON requests with `Content-Type: application/json` because AdGuard’s API tightened request validation in recent releases.
- **Plugin boundaries:** keep source/output interfaces internal to the module for MVP; expose extension points by package design, not process boundaries.
## Installation
# Core runtime dependencies
# Optional but recommended for retry policy
# Test-only dependencies
# Tooling
## Sources
- Go stable version text (`go1.26.3` on 2026-05-12): https://go.dev/VERSION?m=text — HIGH confidence
- Go 1.26 release notes: https://go.dev/doc/go1.26 — HIGH confidence
- Docker SDK examples using the Moby Go client: https://docs.docker.com/reference/api/engine/sdk/examples/ — HIGH confidence
- Docker Engine API v1.54 reference: https://docs.docker.com/reference/api/engine/version/v1.54/ — HIGH confidence
- Docker daemon access/security guidance: https://docs.docker.com/engine/security/protect-access/ — HIGH confidence
- Moby Go client README: https://github.com/moby/moby/tree/master/client — HIGH confidence
- AdGuard Home technical document, rewrite API semantics: https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/AGHTechDoc.md — HIGH confidence
- AdGuard Home OpenAPI changelog, rewrite update/settings changes through `v0.107.72`: https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/openapi/CHANGELOG.md — HIGH confidence
- AdGuard Home OpenAPI spec: https://github.com/AdguardTeam/AdGuardHome/blob/master/openapi/openapi.yaml — HIGH confidence
- GoDoxy README, label behavior and watch/update model: https://raw.githubusercontent.com/yusing/godoxy/main/README.md — HIGH confidence
- BurntSushi TOML README/releases: https://github.com/BurntSushi/toml — HIGH confidence
- Testcontainers for Go docs/release page: https://github.com/testcontainers/testcontainers-go — HIGH confidence
- cenkalti/backoff README: https://github.com/cenkalti/backoff — MEDIUM confidence
- Module versions validated directly on 2026-05-12 via `go list -m ...@latest` against the Go module proxy: `github.com/moby/moby/client v0.4.1`, `github.com/BurntSushi/toml v1.6.0`, `github.com/testcontainers/testcontainers-go v0.42.0`, `github.com/cenkalti/backoff/v5 v5.0.3`, `github.com/goreleaser/goreleaser/v2 v2.15.4`, `github.com/golangci/golangci-lint/v2 v2.12.2` — HIGH confidence
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

Conventions not yet established. Will populate as patterns emerge during development.
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## Architecture

Architecture not yet mapped. Follow existing patterns found in the codebase.
<!-- GSD:architecture-end -->

<!-- GSD:skills-start source:skills/ -->
## Project Skills

No project skills found. Add skills to any of: `.claude/skills/`, `.agents/skills/`, `.cursor/skills/`, `.github/skills/`, or `.codex/skills/` with a `SKILL.md` index file.
<!-- GSD:skills-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd-quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd-debug` for investigation and bug fixing
- `/gsd-execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->



<!-- GSD:profile-start -->
## Developer Profile

> Profile not yet configured. Run `/gsd-profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
