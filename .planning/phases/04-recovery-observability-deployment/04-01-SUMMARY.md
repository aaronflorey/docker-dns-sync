---
phase: 04-recovery-observability-deployment
plan: 01
subsystem: runtime
tags: [docker, watch, reconnect, reconciliation]
requires:
  - phase: 03-docker-godoxy-snapshot-automation
    provides: startup snapshot reconciliation over Docker desired state
provides:
  - Optional live-watch source contract alongside snapshot sources
  - Docker event watch hints with reconnect-safe runtime handling, including network attach/detach changes
  - Steady-state runtime reconcile loop that reuses startup reconciliation
affects: [phase-04-recovery, runtime-loop, docker-watch]
tech-stack:
  added: []
  patterns: [optional source watch contract, reconcile-on-hint runtime loop]
key-files:
  created: []
  modified:
    - internal/contracts/source.go
    - internal/providers/docker/source.go
    - internal/providers/docker/source_test.go
    - internal/runtime/app.go
    - internal/runtime/app_test.go
key-decisions:
  - "Treat Docker events as coarse reconcile hints instead of direct mutation instructions."
  - "Recover from watch disconnects by rerunning reconciliation before resuming the stream, and back off instead of exiting if Docker is briefly unavailable during repair."
patterns-established:
  - "Startup-only sources remain valid; watch participation is optional through a sibling interface."
  - "Runtime steady state reuses the same reconcile-and-persist path as startup."
requirements-completed: [STATE-01, STATE-03]
duration: 0 min
completed: 2026-05-13
---

# Phase 4 Plan 1: Watch And Reconnect Summary

**Added the first long-running runtime loop so the daemon now keeps reconciling after startup and repairs itself after Docker watch disconnects without hot-looping repeated reconnect failures.**

## Accomplishments
- Added `contracts.WatchableSource` as an optional live-watch contract adjacent to the required snapshot source interface.
- Implemented Docker event watch support that emits generic reconcile hints for relevant container events, including network connect/disconnect changes.
- Extended the runtime to keep running after startup, react to hints, rerun reconciliation before restarting a failed watch, and apply reconnect backoff so repeated disconnects do not hot-loop while Docker is unstable.
- Added runtime and Docker provider tests covering hint-driven reconcile, reconnect recovery, and transient Docker unavailability during repair.

## Files Created/Modified
- `internal/contracts/source.go` - optional watch contract for sources.
- `internal/providers/docker/source.go` - Docker watch implementation with conservative event filtering.
- `internal/providers/docker/source_test.go` - fake stream tests for hint emission and disconnect propagation.
- `internal/runtime/app.go` - steady-state watch orchestration and reconnect handling.
- `internal/runtime/app_test.go` - runtime-loop tests for hint-driven reconcile and reconnect repair.

## Deviations from Plan

None.

## Issues Encountered

None.

## Self-Check: PASSED
