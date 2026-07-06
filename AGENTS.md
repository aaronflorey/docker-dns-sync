# AGENTS.md

## Project Overview

`docker-dns-sync` is a Go daemon. The CLI entrypoint is `cmd/docker-dns-sync`; runtime code lives under `internal/` and reconciles Docker-derived desired DNS records into output providers while preserving operator-managed records.

## Source Of Truth

- Toolchain and tasks: `mise.toml`, `go.mod`
- CI checks: `.github/workflows/ci.yaml`
- Release automation: `.github/workflows/release.yaml`, `.goreleaser.yaml`, `release-please-config.json`, `.release-please-manifest.json`
- Operator docs and config examples: `README.md`, `config.example.toml`, `testdata/config/*.toml`
- Live smoke harness: `deploy/compose/live-test/README.md`, `deploy/compose/live-test/verify.sh`, `deploy/compose/live-test/compose.yaml`

## Commands

| Purpose | Command | When to run |
|---|---|---|
| Install pinned Go | `mise install` | Fresh checkout or toolchain drift |
| List packages | `mise exec -- go list ./...` | After package/layout changes |
| Run all unit tests | `mise exec -- go test ./...` | Before final handoff for code changes |
| Run one package | `mise exec -- go test ./internal/runtime` | While iterating on one package |
| Run one test | `mise exec -- go test ./internal/runtime -run TestName` | Focused debugging |
| Static checks | `mise exec -- go vet ./...` | Matches CI before handoff |
| Format Go files | `mise exec -- gofmt -w <files>` | After editing Go |
| Build binary | `mise exec -- go build -o bin/docker-dns-sync ./cmd/docker-dns-sync` | CLI or release-impacting changes |
| Live smoke test | `mise run live-test` | Docker/AdGuard/reconcile integration changes |
| Keep live stack | `mise run live-test -- --keep-running` | Only when inspecting the live harness |
| CI container build | `docker build -t docker-dns-sync-ci .` | Dockerfile/runtime image changes |

CI also runs GoReleaser config validation via `goreleaser check`; run it locally only if GoReleaser v2 is installed.

## Workflow

- Existing repo instructions require starting edits through a GSD command when available: `/gsd-quick` for small changes, `/gsd-debug` for bug investigations, `/gsd-execute-phase` for planned phase work.
- Prefer `mise exec -- ...` for Go tool commands so the pinned `go1.26.3` toolchain is used.
- Use the smallest relevant package test first, then `go vet ./...` and `go test ./...` before final handoff when code changed.

## Architecture Notes

- Config loading is strict TOML in `internal/config/load.go`; unknown keys are errors.
- The CLI only accepts `-config <path>` and returns an error when it is omitted.
- Runtime startup flow in `internal/runtime/app.go`: validate config, resolve secrets, build dependencies/providers, create the state store, run startup reconcile, then watch sources for reconcile hints.
- Provider contracts are in `internal/contracts`; the default registry wires source `docker` and outputs `adguard` and `cloudflare` in `internal/runtime/factories.go`.
- Reconciliation safety is central: only mutate records supported by daemon-owned state/provenance, persist state after successful mutations, and preserve ambiguous/manual records instead of guessing.
- AdGuard visible rewrite listings do not expose unique provenance, so same-key stale cleanup must stay conservative; the live test intentionally skips destructive stale-delete assertions for AdGuard.
- State is a local JSON ownership snapshot written through `internal/state.Store` with `0600` atomic writes.

## Config And Provider Gotchas

- Output secrets should use refs such as `password_ref = "ENV:ADGUARD_PASSWORD"` or `api_key_ref = "ENV:CLOUDFLARE_API_KEY"`; do not add examples with real inline secrets.
- Docker sources accept `unix://` or `tcp://` endpoints. For local socket deployments, set `sources[].host_ip` unless each record has an explicit host override label.
- Docker label derivation is GoDoxy-compatible: `proxy.aliases` plus `proxy.<alias>.port`, `proxy.#<n>.port`, or `proxy.*.port`; `proxy.exclude=true` disables a container and `proxy.dns=false` disables DNS derivation.
- `proxy.dns=<output-type>` routes a desired record to one output type; blank/true records are eligible for all enabled outputs.

## Testing

- Unit tests are Docker-free and live next to packages as `*_test.go`.
- The live harness requires `docker`, `docker compose`, and `curl`; `dig` is optional because the script falls back to containerized `nslookup`.
- `mise run live-test` builds and runs the compose stack, verifies create/update/manual-record restart safety, and cleans up with `docker compose ... down -v` plus temporary runtime directory removal.
- Keep-running mode preserves temporary runtime dirs and prints cleanup commands; do not leave that stack running after inspection.

## Security And Operations

- Never log or print secret values, environment contents, AdGuard passwords, or Cloudflare API keys. Avoid `set -x` in scripts touching secrets.
- Mounting `/var/run/docker.sock` gives broad Docker control; do not weaken the README warnings or compose examples around socket access.
- The root `docker-compose.yaml` expects `ADGUARD_PASSWORD` from the shell or `.env` and persists state in `./state`.
- Release publishing requires the `HOMEBREW_TAP_GITHUB_TOKEN` secret and is handled by Release Please plus GoReleaser on release tags.

## Dependencies

- Use Go modules only; keep `go.mod` and `go.sum` together.
- Runtime dependencies are intentionally small. Do not add a new dependency for simple stdlib-covered logic.
- Renovate manages Go modules, GitHub Actions, and container image references via `renovate.json`.
