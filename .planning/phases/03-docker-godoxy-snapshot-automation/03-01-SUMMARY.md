---
phase: 03-docker-godoxy-snapshot-automation
plan: 01
subsystem: source
tags: [docker, godoxy, labels, desired-state]
requires:
  - phase: 02-ownership-safe-reconciliation-core
    provides: ownership-safe runtime reconcile and persisted lineage model
provides:
  - Narrow Godoxy label subset helpers for desired-record derivation
  - Deterministic provider-level tests for aliases, exclusion, wildcard/reference forms, host precedence, and preserved `#N` alias positions
affects: [phase-03-snapshot-source, phase-04-docker-watch-recovery]
tech-stack:
  added: []
  patterns: [pure provider-local label helpers, deterministic desired-record sorting]
key-files:
  created:
    - internal/providers/docker/labels.go
    - internal/providers/docker/labels_test.go
    - internal/providers/docker/source_test.go
key-decisions:
  - "Mirror only the MVP DNS-relevant Godoxy subset instead of importing the full upstream parser."
  - "Use alias-specific host override first, then #N, then wildcard, before the derived default target."
patterns-established:
  - "Docker label expansion stays local to the provider and returns normalized contracts.DesiredRecord values only."
requirements-completed: [SRC-02, SRC-03]
duration: 0 min
completed: 2026-05-13
---

# Phase 3 Plan 1: Docker Label Subset Summary

**Locked the Phase 3 Godoxy DNS subset in provider-local helpers and deterministic tests before wiring the full Docker snapshot provider.**

## Accomplishments
- Added `internal/providers/docker/labels.go` with narrow alias, exclusion, host-override, and default-target derivation helpers.
- Added compatibility tests covering alias fallback, `proxy.aliases`, `proxy.#N.port`, `proxy.*.port`, wildcard/reference host overrides, exclusion, and preserved indexed alias positions after filtering.
- Added provider-fixture tests that exercise desired-record derivation through the Docker provider seam.

## Files Created/Modified
- `internal/providers/docker/labels.go` - narrow Godoxy subset parsing and desired-record derivation.
- `internal/providers/docker/labels_test.go` - deterministic subset compatibility coverage.
- `internal/providers/docker/source_test.go` - container summary fixtures for provider-level desired output.

## Decisions Made
- Kept the compatibility surface intentionally narrow to the documented DNS subset needed for MVP startup reconciliation.
- Preserved original `proxy.aliases` positions for `#N` overrides even when unsupported aliases are filtered out.
- Sorted final desired records to keep tests and future reconcile input stable.

## Deviations from Plan

None.

## Issues Encountered

None.

## Self-Check: PASSED
