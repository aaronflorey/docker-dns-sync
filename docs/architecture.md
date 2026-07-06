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

That comparison decides whether to create, update, delete, or drop records. Destructive cleanup is gated on unique visible daemon-owned provenance: the planner only deletes a stale remote record when exactly one same-key visible record exposes non-empty daemon-owned proof. Same-key matches without provenance, ambiguous duplicates, and unmanaged/manual records are preserved, so label changes do not imply remote cleanup unless the provider can prove ownership. Only daemon-owned records are persisted back into state.

## Recovery behavior

- Startup always performs a full reconcile from current Docker snapshot + state.
- Docker watch events are treated as hints, not source of truth.
- Provider read/write calls during reconcile run with the configured per-operation timeout (`runtime.operation_timeout`, default `10s`).
- That timeout applies to `ListDesired`, `ListVisible`, `Create`, `Update`, and `Delete`, but not to long-lived `Watch(ctx)` streams and not to the reconcile loop as a whole.
- Watch disconnects trigger reconnect attempts with bounded backoff.
- Temporary AdGuard output failures also retry within the configured retry window. Retry timing starts only after the attempted operation returns or times out.
- Partial mutations can still be persisted so ownership remains recoverable after an interrupted run.

## State and ownership

The state file tracks managed records by output/source lineage, not by raw rewrite contents alone. Recovery still depends on what the output can prove at reconcile time: stored ownership alone is not enough to delete a stale remote record unless the provider also surfaces matching daemon-owned provenance. This keeps operator-managed DNS safe even when a provider cannot expose stable remote ownership markers.
