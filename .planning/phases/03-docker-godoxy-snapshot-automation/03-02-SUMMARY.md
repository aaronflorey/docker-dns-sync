---
phase: 03-docker-godoxy-snapshot-automation
plan: 02
subsystem: source
tags: [docker, snapshot, runtime, startup-reconcile]
requires:
  - phase: 03-docker-godoxy-snapshot-automation
    provides: locked Docker/Godoxy label subset helpers
provides:
  - Real Docker snapshot ListDesired implementation
  - Deterministic running-container filtering and desired-record output
  - Startup reconcile coverage preserved through the existing runtime path
affects: [phase-03-startup-sync, phase-04-recovery]
tech-stack:
  added: []
  patterns: [provider-owned metadata translation, runtime-owned startup reconciliation]
key-files:
  created: []
  modified:
    - internal/providers/docker/source.go
    - internal/providers/docker/source_test.go
key-decisions:
  - "Filter to running containers in the provider snapshot and skip unsupported cases conservatively."
  - "Preserve runtime startup orchestration unchanged and feed it normalized DesiredRecord output only."
patterns-established:
  - "Docker snapshot listing stays a narrow provider seam over the existing Moby client."
requirements-completed: [SRC-01, RECON-01]
duration: 0 min
completed: 2026-05-13
---

# Phase 3 Plan 2: Docker Snapshot Provider Summary

**Implemented the real Docker snapshot source so startup reconciliation now consumes current labeled container state without any runtime contract changes.**

## Accomplishments
- Expanded the internal Docker client seam just enough to list current containers.
- Implemented `Provider.ListDesired` to list running containers, derive supported desired records, and return them deterministically.
- Preserved the existing runtime startup reconcile path and verified it with targeted runtime tests.
- Tightened target derivation so Docker sources use explicit host overrides first, then the configured non-local endpoint host, and never fall back to container IPs.

## Files Created/Modified
- `internal/providers/docker/source.go` - real Docker snapshot `ListDesired` implementation.
- `internal/providers/docker/source_test.go` - fake-client coverage for listing, filtering, and deterministic desired output.
- `internal/runtime/app_test.go` - existing startup reconcile coverage reused unchanged.
- `internal/runtime/factories_test.go` - existing endpoint wiring coverage reused unchanged.

## Decisions Made
- Kept Docker metadata translation local to the provider rather than introducing any new startup path or config knobs.
- Used the configured Docker endpoint host as the only default answer target and require explicit host overrides when a local endpoint has no host component.

## Deviations from Plan

None.

## Issues Encountered

None.

## Self-Check: PASSED
