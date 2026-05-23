# Architecture

## High-level flow

1. `cmd/docker-dns-sync/main.go` parses `-config` and starts the app.
2. `internal/config` loads TOML, validates it, and resolves `ENV:` secret references.
3. `internal/runtime` builds source and output providers from the registry.
4. `internal/state` opens or creates the owned-record snapshot file.
5. The app performs a startup reconcile, then enters steady-state watch/reconcile mode.

## Main packages

| Package | Responsibility |
| --- | --- |
| `cmd/docker-dns-sync` | CLI entrypoint and signal handling. |
| `internal/config` | TOML model, validation, and secret resolution. |
| `internal/runtime` | Provider wiring, reconciliation, retries, and watch handling. |
| `internal/providers/docker` | Docker snapshot and event source. |
| `internal/providers/adguard` | AdGuard rewrite output. |
| `internal/providers/cloudflare` | Cloudflare DNS output. |
| `internal/state` | Persisted ownership snapshot. |

## Reconciliation model

The daemon does not blindly mirror every visible record. It compares:

- desired records from Docker sources,
- visible records from each output,
- owned records from the state snapshot.

That comparison decides whether to create, update, delete, or drop records. Only daemon-owned records are persisted back into state.

## Recovery behavior

- Startup always performs a full reconcile from current Docker snapshot + state.
- Docker watch events are treated as hints, not source of truth.
- Watch disconnects trigger reconnect attempts with bounded backoff.
- Temporary AdGuard output failures also retry within the configured retry window.
- Partial mutations can still be persisted so ownership remains recoverable after an interrupted run.

## State and ownership

The state file tracks managed records by output/source lineage, not by raw rewrite contents alone. This lets the daemon recover safely after restarts without touching records it does not own.
