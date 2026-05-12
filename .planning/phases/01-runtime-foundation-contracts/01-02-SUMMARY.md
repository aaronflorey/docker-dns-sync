---
phase: 01-runtime-foundation-contracts
plan: 02
subsystem: infra
tags: [go, toml, config, validation, secrets]
requires: [CONF-01, CONF-02, CONF-03]
provides:
  - Semantic startup validation for runtime-critical config sections
  - Safe env-backed secret resolution without leaking resolved values
  - Config coverage for Docker local-socket and socket-proxy endpoint modes
affects: [phase-01, phase-03, phase-04]
tech-stack:
  added: []
  patterns: [two-phase config validation, env-backed secret references]
key-files:
  created: [internal/config/validate.go, internal/config/secrets.go, internal/config/validate_test.go, testdata/config/socket-proxy.toml, testdata/config/env-secret.toml]
  modified: [internal/config/model.go, internal/config/load.go, testdata/config/minimal.toml]
key-decisions:
  - "Require exactly one of password or password_ref so credential sourcing is explicit and unambiguous."
  - "Resolve password_ref values via ENV: references during config load and clear the ref from the returned runtime config copy."
patterns-established:
  - "Config load now follows decode -> validate -> resolve-secrets before runtime startup"
  - "Docker source endpoints are validated as explicit unix:// or tcp:// addresses"
requirements-completed: [CONF-02, CONF-03]
duration: 10 min
completed: 2026-05-12
---

# Phase 1 Plan 2: Runtime Foundation & Contracts Summary

**Startup config is now semantically validated, supports explicit env-backed secret references, and accepts both local-socket and socket-proxy Docker endpoint forms.**

## Performance

- **Duration:** 10 min
- **Completed:** 2026-05-12T23:25:00Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- Added requirement-focused config tests for missing source/output blocks, runtime field validation, Docker endpoint modes, and secret resolution.
- Implemented `Validate` to reject unusable startup config before runtime wiring begins.
- Implemented `ResolveSecrets` so `password_ref = "ENV:..."` can be resolved safely without logging or embedding secret values in errors.
- Updated config fixtures so minimal startup uses an inline test password while env-backed secret behavior is covered separately.

## Files Created/Modified
- `internal/config/model.go` - Added explicit `password` and `password_ref` config fields.
- `internal/config/load.go` - Switched startup loading to decode, validate, then resolve secrets.
- `internal/config/validate.go` - Added semantic validation for sources, outputs, runtime fields, and retry durations.
- `internal/config/secrets.go` - Added env-backed secret resolution with non-mutating config copying.
- `internal/config/validate_test.go` - Added focused requirement-level tests for config behavior.
- `testdata/config/minimal.toml` - Moved the baseline startup fixture to inline password auth.
- `testdata/config/socket-proxy.toml` - Added a concrete TCP socket-proxy fixture.
- `testdata/config/env-secret.toml` - Added a concrete `ENV:` secret-reference fixture.

## Decisions Made
- Kept Docker endpoint validation narrow to the Phase 1 requirement: accept `unix://` and `tcp://` forms only.
- Cleared `password_ref` after resolution in the returned config copy so runtime code carries concrete credentials, not unresolved refs.

## Deviations from Plan
None.

## Issues Encountered
- `ResolveSecrets` initially mutated the caller's output slice through the copied config struct; fixed by cloning the outputs slice before applying resolved secrets.

## User Setup Required
None.

## Next Phase Readiness
- Config invariants are now stable enough for provider factory wiring work in `01-03`.
- Runtime startup can safely consume inline or env-backed credentials without leaking secret values.

## Self-Check: PASSED

---
*Phase: 01-runtime-foundation-contracts*
*Completed: 2026-05-12*
