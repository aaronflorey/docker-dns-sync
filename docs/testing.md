# Testing

## Default test command

```bash
mise exec -- go test ./...
```

The default test path is Docker-free. The CI workflow uses the same unit-test command and does not require the live Docker stack.

## Static verification

```bash
mise exec -- go vet ./...
```

`go vet` is also part of the normal Docker-free verification path.

## Opt-in live Docker verification

Use the live test stack in `deploy/compose/live-test/` only when you want end-to-end coverage against Docker + AdGuard Home.

```bash
mise run live-test
```

Prerequisites:

- `docker`
- `docker compose`
- `curl`
- Optional: `dig` if you want host-side DNS tooling. The script falls back to containerized `nslookup` when `dig` is unavailable.

What `mise run live-test` covers:

- managed rewrite create/update/restore
- manual-record safety
- restart recovery

Cleanup behavior:

- default: tears the stack down with `docker compose ... down -v`
- opt-out: `mise run live-test -- --keep-running` or `KEEP_RUNNING=1 mise run live-test`
- keep-running mode preserves the temporary runtime bind-mount directories and prints the exact compose-down plus `rm -rf ...` cleanup commands you must run after inspection

Secret and log hygiene:

- keep credentials in env references or fixture defaults; do not paste real credentials inline
- do not echo secret env values, enable shell tracing, or dump compose logs by default

Coverage note: stale-owned remote rewrite deletion is intentionally not asserted here until `.weave/plans/stale-owned-remote-cleanup.md` lands.

## Test fixtures

- `testdata/config/minimal.toml` — minimal runtime config.
- `testdata/config/example.toml` — host-oriented example.
- `testdata/config/socket-proxy.toml` — remote/proxy Docker example.
- `testdata/config/env-secret.toml` — secret-reference example.
- `testdata/config/docker-container.toml` — container deployment example.

## Adding tests

Prefer repository-level behavior tests for reconciliation, provider behavior, and config validation. The codebase already includes explicit tests around validation, provider factories, and live reconcile flows.
