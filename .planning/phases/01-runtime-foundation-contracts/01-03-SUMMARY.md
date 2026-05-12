---
phase: 01-runtime-foundation-contracts
plan: 03
subsystem: infra
tags: [go, contracts, runtime, docker, factories]
requires: [OPS-03, CONF-01, CONF-03]
provides:
  - Stable source and output interfaces with immutable source-object identity
  - Runtime-owned provider registry and factory construction seams
  - Real Docker client bootstrap from configured endpoint values
affects: [phase-01, phase-02, phase-03]
tech-stack:
  added: [github.com/moby/moby/client]
  patterns: [runtime-owned factories, narrow provider contracts]
key-files:
  created: [internal/contracts/source.go, internal/contracts/output.go, internal/providers/docker/source.go, internal/providers/adguardstub/output.go, internal/runtime/factories.go, internal/runtime/factories_test.go]
  modified: [go.mod, go.sum, internal/runtime/app.go]
key-decisions:
  - "Keep provider contracts narrow: sources list desired records and outputs expose visible-state CRUD without embedding reconciliation policy."
  - "Make runtime own provider registration and construction instead of allowing package-level self-registration."
patterns-established:
  - "Provider type strings map to explicit runtime factories"
  - "Docker source bootstrap uses validated endpoint config to construct a real SDK client without starting discovery work yet"
requirements-completed: [OPS-03]
duration: 11 min
completed: 2026-05-12
---

# Phase 1 Plan 3: Runtime Foundation & Contracts Summary

**The runtime now owns source/output construction through stable contract interfaces, and the Docker source can be bootstrapped from either local-socket or proxy endpoint config.**

## Performance

- **Duration:** 11 min
- **Completed:** 2026-05-12T23:31:00Z
- **Tasks:** 2
- **Files modified:** 9

## Accomplishments
- Added stable contract types for immutable source identity, desired records, visible records, and item-level output operations.
- Added a registry-backed runtime factory layer that constructs providers from config instead of relying on globals or `init()` registration.
- Added a real Docker bootstrap package that builds an SDK client from configured endpoint values while keeping discovery logic deferred to Phase 3.
- Wired runtime startup through the new factory registry so provider construction failures happen on the normal daemon run path.

## Files Created/Modified
- `internal/contracts/source.go` - Source contract types and immutable source-object identity model.
- `internal/contracts/output.go` - Output contract and visible-record types for later reconcile work.
- `internal/providers/docker/source.go` - Docker source bootstrap package using `client.NewClientWithOpts`.
- `internal/providers/adguardstub/output.go` - Stub AdGuard output package for constructor seam coverage.
- `internal/runtime/factories.go` - Provider registries plus config-driven source/output construction.
- `internal/runtime/factories_test.go` - Extensibility and configured-endpoint tests for the runtime factory layer.
- `internal/runtime/app.go` - Runtime startup now builds providers before entering the long-running loop.
- `go.mod` / `go.sum` - Added the Moby Docker client dependency and transitive module locks.

## Decisions Made
- Kept provider registration explicit in `runtime` so future providers add a factory entry instead of modifying contracts.
- Limited the Docker provider to client construction and identity wiring only, avoiding premature snapshot/watch logic in this slice.

## Deviations from Plan
None.

## Issues Encountered
None.

## User Setup Required
None.

## Next Phase Readiness
- Phase 1 now has the config and provider seams needed for atomic state-store wiring in `01-04`.
- Later Docker and AdGuard implementations can plug into the locked contracts without changing the runtime boundary.

## Self-Check: PASSED

---
*Phase: 01-runtime-foundation-contracts*
*Completed: 2026-05-12*
