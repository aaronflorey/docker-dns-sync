# Phase 2: Ownership-Safe Reconciliation Core - Discussion Log (Assumptions Mode)

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions captured in CONTEXT.md - this log preserves the analysis.

**Date:** 2026-05-12
**Phase:** 02-ownership-safe-reconciliation-core
**Mode:** assumptions
**Areas analyzed:** Ownership Mutation Boundary, Record Identity For Diffing, State Update Ordering, Reconciliation Placement And Scope

## Assumptions Presented

### Ownership Mutation Boundary
| Assumption | Confidence | Evidence |
|------------|-----------|----------|
| Reconciliation should treat persisted `state.ManagedRecord` entries as the only authority for destructive mutations, meaning visible AdGuard rewrites that are not present in local state are ignored rather than updated or deleted. | Confident | `.planning/ROADMAP.md`, `.planning/REQUIREMENTS.md`, `.planning/STATE.md`, `internal/state/model.go` |

### Record Identity For Diffing
| Assumption | Confidence | Evidence |
|------------|-----------|----------|
| Managed and visible rewrites should be matched by normalized `hostname/domain + answer` within an output provider, not by any AdGuard-native record ID, because AdGuard does not expose one. Duplicate remote entries for the same pair should be treated as ambiguous and reconciled conservatively. | Confident | `internal/contracts/output.go`, `internal/state/model.go`, AdGuard `openapi.yaml`, AdGuard `openapi/CHANGELOG.md`, `internal/filtering/rewritehttp.go`, `internal/filtering/rewrites.go` |

### State Update Ordering
| Assumption | Confidence | Evidence |
|------------|-----------|----------|
| The reconciler should persist ownership state only after successful AdGuard item mutations and should update `LastAppliedAt` from applied results, not before. | Confident | `internal/state/store.go`, `internal/state/model.go`, `.planning/research/ARCHITECTURE.md` |

### Reconciliation Placement And Scope
| Assumption | Confidence | Evidence |
|------------|-----------|----------|
| Phase 2 should introduce a centralized runtime/core reconciler that performs full-snapshot diffing from `ListDesired`, `ListVisible`, and persisted state, rather than embedding reconcile behavior inside source/output providers. | Likely | `.planning/phases/01-runtime-foundation-contracts/01-CONTEXT.md`, `.planning/STATE.md`, `internal/contracts/source.go`, `internal/contracts/output.go`, `internal/runtime/app.go`, `.planning/research/ARCHITECTURE.md` |

## Corrections Made

No corrections - all assumptions confirmed.

## External Research

- Real AdGuard Home rewrite API identifies rewrite items by value, not remote ID; `update` matches a rewrite object and preserves `enabled` when omitted. (Source: https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/openapi/openapi.yaml, https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/openapi/CHANGELOG.md)
- Server-side matching uses only `domain + answer`, not `enabled`, and `delete` removes all exact matches. (Source: https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/internal/filtering/rewritehttp.go, https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/internal/filtering/rewrites.go)
- AdGuard allows duplicate rewrite entries and does not expose a stronger remote identity, so Phase 2 must own only unique `(domain, answer)` pairs and treat duplicate remote state as ambiguous/unsafe. (Source: https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/internal/filtering/rewritehttp.go, https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/internal/filtering/rewrite/storage.go)
