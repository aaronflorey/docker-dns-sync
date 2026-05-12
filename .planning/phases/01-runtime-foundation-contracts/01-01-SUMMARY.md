---
phase: 01-runtime-foundation-contracts
plan: 01
subsystem: infra
tags: [go, toml, runtime, cli, config]
requires: []
provides:
  - Go 1.26.3-pinned daemon module with a runnable CLI startup path
  - Root TOML config contract and loader with fail-fast startup validation
  - Cancellable runtime loop skeleton for subsequent reconciliation work
affects: [phase-02, phase-03, phase-04]
tech-stack:
  added: [github.com/BurntSushi/toml]
  patterns: [config-before-runtime bootstrap, cancellable long-running app loop]
key-files:
  created: [go.mod, mise.toml, cmd/docker-dns-sync/main.go, internal/config/model.go, internal/config/load.go, internal/runtime/app.go, testdata/config/minimal.toml, testdata/config/malformed.toml]
  modified: [cmd/docker-dns-sync/main_test.go]
key-decisions:
  - "Expose runWithContext for deterministic cancellation testing while keeping signal-based run() for production CLI flow."
  - "Enforce source/output presence in config.Load so bootstrap fails before entering runtime."
patterns-established:
  - "Minimal runtime skeleton: load config, construct app, block on context cancellation"
  - "Phase fixtures define startup contract up front with full sections"
requirements-completed: [CONF-01]
duration: 12 min
completed: 2026-05-12
---

# Phase 1 Plan 1: Runtime Foundation & Contracts Summary

**Config-driven daemon startup now loads TOML, validates required source/output blocks, and enters a cleanly cancellable runtime loop.**

## Performance

- **Duration:** 12 min
- **Started:** 2026-05-12T23:05:00Z
- **Completed:** 2026-05-12T23:17:42Z
- **Tasks:** 2
- **Files modified:** 10

## Accomplishments
- Added startup test scaffolding and locked minimal/malformed TOML fixtures for the Phase 1 config contract.
- Implemented module/toolchain pinning plus the first real CLI startup path (`-config`) to load TOML and start runtime.
- Added real startup behavior assertions for happy path, missing config rejection, malformed TOML rejection, and cancellation blocking semantics.

## Task Commits

1. **Task 1: Create startup test scaffolding and Phase 1 config fixtures** - `3c7a09d` (test)
2. **Task 2: Implement the minimal config-driven CLI and cancellable runtime loop** - `cf45aca` (feat)

## Files Created/Modified
- `cmd/docker-dns-sync/main_test.go` - Startup contract tests with real assertions and cancellation checks.
- `testdata/config/minimal.toml` - Full minimal Phase 1 startup fixture with sources/outputs/state/logging/retry sections.
- `testdata/config/malformed.toml` - Intentionally invalid TOML fixture for bootstrap rejection tests.
- `go.mod` - New Go module pinned to `go 1.26` and `toolchain go1.26.3`.
- `mise.toml` - Toolchain pin for reproducible local execution.
- `cmd/docker-dns-sync/main.go` - CLI parsing and bootstrap handoff (`config.Load` → `runtime.New` → `App.Run`).
- `internal/config/model.go` - Root runtime config model and nested sections.
- `internal/config/load.go` - TOML loader with fail-fast checks for path/read/decode and source/output presence.
- `internal/runtime/app.go` - Thin long-running loop that blocks on context cancellation.

## Decisions Made
- Added `runWithContext` for tests to control cancellation directly without process-level signal coupling.
- Kept runtime behavior intentionally minimal (startup log + `<-ctx.Done()>`) to avoid pulling reconciliation/provider work into this plan.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Generated missing dependency lock entries**
- **Found during:** Task 2
- **Issue:** Targeted Go tests failed because `go.sum` entries for `github.com/BurntSushi/toml` were missing.
- **Fix:** Ran `mise exec -- go mod tidy` to materialize module checksums.
- **Files modified:** `go.sum`
- **Verification:** Targeted startup tests and full `go test ./...` both passed afterward.
- **Committed in:** `cf45aca`

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Required for a reproducible and buildable module; no scope creep.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Runtime bootstrap contract is now executable and test-covered.
- Ready for 01-02 semantic config validation and secret-resolution expansion.

## Self-Check: PASSED

---
*Phase: 01-runtime-foundation-contracts*
*Completed: 2026-05-12*
