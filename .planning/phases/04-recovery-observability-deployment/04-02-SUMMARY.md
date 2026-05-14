---
phase: 04-recovery-observability-deployment
plan: 02
subsystem: runtime
tags: [retry, logging, adguard, persistence]
requires:
  - phase: 04-recovery-observability-deployment
    provides: steady-state watch loop and reconnect repair path
provides:
  - Bounded full-reconcile retries for temporary output visibility read and write failures
  - Structured logs for reconcile lifecycle, retries, output writes, and state persistence
  - Runtime tests proving retries do not persist speculative state
affects: [phase-04-observability, adguard-recovery, persistence-safety]
tech-stack:
  added: []
  patterns: [runtime-owned retry wrapper, structured operation-boundary logging]
key-files:
  created: []
  modified:
    - internal/runtime/app.go
    - internal/runtime/deps.go
    - internal/runtime/reconcile.go
    - internal/runtime/app_test.go
key-decisions:
  - "Keep retry and logging policy in runtime by wrapping outputs instead of moving logic into providers."
  - "Log at operation boundaries and state persistence boundaries without exposing secret values."
patterns-established:
  - "Temporary output read and write failures retry from the full reconcile boundary so each attempt re-reads state, sources, and outputs before any new mutation planning."
  - "Create, update, and delete remain single-shot within each reconcile attempt so partial remote writes are recovered by the next full pass instead of inline RPC replay."
  - "State persistence is logged from the reconcile boundary after successful apply only."
requirements-completed: [STATE-04, OPS-01]
duration: 0 min
completed: 2026-05-13
---

# Phase 4 Plan 2: Retry And Logging Summary

**Bounded read retries, partial-progress persistence, and structured runtime logging keep recovery visible without automatically replaying remote mutations.**

## Accomplishments
- Added bounded runtime retries at the full reconcile boundary for transient AdGuard `ListVisible` and write failures using the existing retry config.
- Kept output wrappers focused on mutation-success logging so `Create`, `Update`, and `Delete` remain single-shot within each attempt while each retry starts from fresh state, source, and output reads.
- Persisted partial reconcile progress after later mutation failure so successfully applied managed records are not orphaned from daemon ownership state.
- Retried transient source `ListDesired` failures during watch-triggered full reconciles, including the startup handoff resync, even when the temporary read failure comes from a different configured source than the watch event origin.
- Added a startup handoff resync after watches become active, treated clean watch closure as a disconnect, and restarted disconnected subscriptions before reconnect repair so the runtime does not leave an unobserved gap around startup or reconnects.
- Filtered Docker container-event hints so clearly irrelevant unlabeled activity does not force a full reconcile while labeled container lifecycle changes and network connect/disconnect recovery still do.
- Reset per-source watch reconnect backoff after a successful reconnect repair/handoff so later disconnects start from the configured initial interval instead of a stale escalated delay.
- Logged reconcile start/finish, output mutation success, and state persistence boundaries.
- Made the new runtime test stubs race-safe under `go test -race` and verified both the full suite and the runtime/docker race suite.
- Added tests proving `ListVisible` retries succeed when failures are transient, temporary AdGuard write failures rerun a bounded full reconcile instead of inline mutation retries, partial mutation progress is persisted before a later failure returns, the watch startup handoff resync retries transient source failures, clean closures reconnect with backoff, reconnect repair resets watch backoff, watch-triggered source-read retries recover even when a different source fails transiently, and irrelevant container events are filtered out.

## Files Created/Modified
- `internal/runtime/app.go` - watch-loop source-read retry handling across any source, startup handoff retry reuse, reconnect backoff, clean-closure disconnect handling, bounded full-reconcile retries for temporary output write failures, mutation logging wrapper, and runtime lifecycle logs.
- `internal/providers/adguard/output.go` - temporary AdGuard transport/server failure classification for bounded runtime retries.
- `internal/providers/docker/source.go` - relevant Docker watch-hint filtering for labeled container lifecycle changes and network connect/disconnect recovery.
- `internal/runtime/reconcile.go` - reconcile-boundary persistence for both successful and partial-progress mutation paths.
- `internal/runtime/reconcile_apply.go` - incremental snapshot mutation tracking during apply.
- `internal/runtime/app_test.go` - reconnect backoff, startup handoff retry, cross-source watch-trigger retry coverage, and bounded full-reconcile retry coverage for temporary write failures.
- `internal/providers/adguard/output_test.go` - retryable temporary-vs-terminal AdGuard error coverage.
- `internal/providers/docker/source_test.go` - Docker hint filtering coverage for irrelevant and labeled container events.
- `internal/runtime/reconcile_state_test.go` - partial-progress persistence coverage.

## Deviations from Plan

None.

## Issues Encountered

None.

## Self-Check: PASSED
