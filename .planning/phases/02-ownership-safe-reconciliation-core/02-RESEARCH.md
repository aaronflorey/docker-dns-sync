# Phase 02: ownership-safe-reconciliation-core - Research

**Researched:** 2026-05-13
**Domain:** Ownership-safe reconciliation for AdGuard rewrite records in a Go daemon
**Confidence:** HIGH

## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Treat persisted `state.ManagedRecord` entries as the only authority for destructive mutations, so visible AdGuard rewrites not represented in local state are ignored rather than updated or deleted.
- **D-02:** Match managed and visible rewrites by normalized `hostname/domain + answer` within an output provider, because AdGuard does not expose a stable remote record ID for rewrite items.
- **D-03:** Treat duplicate remote entries for the same normalized `hostname/domain + answer` pair as ambiguous state and reconcile conservatively instead of assuming safe targeted mutation.
- **D-04:** Persist ownership state only after successful AdGuard item mutations, and update `LastAppliedAt` from applied results rather than before remote writes.
- **D-05:** Add one centralized reconcile engine in the runtime/core layer that diffs source desired state, visible AdGuard state, and persisted ownership state rather than embedding reconcile behavior inside providers.

### the agent's Discretion
- Exact internal package boundaries for the reconcile engine, as long as reconcile semantics remain centralized and provider contracts stay narrow.
- Whether duplicate remote state is surfaced as structured warnings, typed errors, or both, as long as the behavior is non-destructive by default.

### Deferred Ideas (OUT OF SCOPE)
- Docker/Godoxy label parsing breadth and startup desired-state generation stay in Phase 3.
- Event-stream reconnects, retry orchestration, and broader recovery behavior stay in Phase 4.
- Richer operator diagnostics, inspect mode, and deployment hardening stay out of this phase.

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| RECON-02 | Daemon-managed AdGuard rewrites are created/updated/deleted idempotently | Reconcile diff model, normalized identity key, item-level apply ordering |
| RECON-03 | Daemon mutates only rewrites it owns in local state | Ownership boundary rules and delete/update gating |
| RECON-04 | Manual/pre-existing AdGuard rewrites stay untouched unless in local state | Visible-vs-owned separation and conservative duplicate handling |
| STATE-02 | State traces managed rewrites to source/output/last-applied | ManagedRecord schema + state write-after-apply policy |

## Summary

Phase 02 should implement a single runtime-level reconcile engine that computes operations from three inputs: desired records (source), visible rewrites (AdGuard), and owned records (state). This is already aligned with existing contracts (`Source.ListDesired`, `Output.ListVisible/Create/Update/Delete`) and the current runtime ownership boundary where orchestration lives above providers. [VERIFIED: codebase grep]

AdGuard rewrite APIs are item-oriented and do not provide a stable per-record ID in the rewrite list response, so matching by normalized `(domain, answer)` is the practical key for safe diffing. Duplicate remote matches for the same normalized key must be treated as ambiguous and non-destructive to avoid deleting/updating an unintended record. [CITED: https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/AGHTechDoc.md] [CITED: https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/openapi/openapi.yaml]

State trustworthiness is the core safety invariant for this phase: only mutate records that are represented in local ownership state, and persist updated state only after successful remote mutations. The current state layer already supports atomic writes and includes required traceability fields (`Output`, `Source`, `Hostname`, `Answer`, `LastAppliedAt`). [VERIFIED: codebase grep]

**Primary recommendation:** Implement a deterministic reconcile planner/apply pipeline in `internal/runtime` that gates destructive operations by owned state membership and persists state snapshots only after successful apply steps.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Reconcile desired/visible/owned sets | API / Backend | Database / Storage | Runtime process owns orchestration and policy; state file stores ownership truth. |
| Rewrite CRUD against AdGuard | API / Backend | — | Provider performs remote API calls through output contract. |
| Ownership ledger durability | Database / Storage | API / Backend | JSON snapshot persists ownership and traceability; runtime controls write timing. |
| Ambiguity/duplicate protection | API / Backend | — | Safety policy is a reconcile decision, not a provider transport concern. |

## Project Constraints (from AGENTS.md)

- Tech stack is locked to Go daemon + Docker Go SDK + AdGuard HTTP API + TOML config + persisted local state file. [VERIFIED: codebase grep]
- Safety invariant: never modify AdGuard rewrites not represented in daemon state. [VERIFIED: codebase grep]
- Credentials must come from config references/env-backed secrets and never be logged. [VERIFIED: codebase grep]
- Keep reconciliation in runtime orchestration layer; provider contracts stay narrow. [VERIFIED: codebase grep]
- Do not introduce unrelated refactors or compatibility layers during this phase. [VERIFIED: codebase grep]

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go toolchain | 1.26.3 | Runtime/compiler for daemon | Already pinned in project and required for consistent builds. [VERIFIED: codebase grep] |
| Go stdlib (`context`, `time`, `maps/slices` as needed, `log/slog`) | go1.26.3 | Reconcile orchestration, timing, deterministic data handling, logging | Enough for this phase without extra dependencies. [VERIFIED: codebase grep] |
| `internal/contracts` + `internal/runtime` | current repo | Source/output interfaces + orchestration home | Existing architecture already enforces narrow provider interfaces. [VERIFIED: codebase grep] |
| AdGuard rewrite HTTP API (`/control/rewrite/list/add/delete/update`) | openapi 0.107 | External mutation surface | Officially documented item-level endpoints match phase needs. [CITED: https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/openapi/openapi.yaml] |
| `internal/state` atomic JSON snapshot store | current repo | Ownership persistence and traceability | Already provides atomic write path and snapshot version checks. [VERIFIED: codebase grep] |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/moby/moby/client` | v0.4.1 (in go.mod) | Future source side feed (Phase 3+) | Keep untouched this phase; only consume source contract output. [VERIFIED: codebase grep] |
| `github.com/BurntSushi/toml` | v1.6.0 (in go.mod) | Config parsing already in place | No new config model needed for reconcile core. [VERIFIED: codebase grep] |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Runtime-level reconcile engine | Reconcile inside output provider | Violates D-05 and makes ownership policy transport-coupled. |
| Owned-state gate for destructive ops | Mutate based on visible-only diff | Breaks RECON-03/RECON-04 safety boundary. |
| Normalized `(hostname, answer)` key | Synthetic hash-only key disconnected from visible data | Adds complexity without solving AdGuard ID absence. |

**Installation:**
```bash
# No new external dependencies required for Phase 02
```

## Package Legitimacy Audit

No new external packages are required for this phase, so no install-time package legitimacy gate is needed in Phase 02 planning. [VERIFIED: codebase grep]

## Architecture Patterns

### System Architecture Diagram

```text
Desired Records (Source.ListDesired)
                |
                v
         Normalize + Index ----------------------+
                |                                |
Visible Rewrites (Output.ListVisible)            |
                |                                |
                v                                |
         Normalize + Index                       |
                |                                |
Owned Records (state.ManagedRecords)             |
                |                                |
                v                                v
          Ownership Gate + Reconcile Planner (runtime/core)
                |
      +---------+----------+----------------+
      |                    |                |
      v                    v                v
   Create Ops           Update Ops       Delete Ops
      |                    |                |
      +--------- Apply via Output Provider -+
                           |
                           v
         On success: persist new snapshot (atomic Save)
```

### Recommended Project Structure
```text
internal/runtime/
├── reconcile.go           # public runtime entrypoint for one reconcile cycle
├── reconcile_plan.go      # diff planner and operation sets
├── reconcile_apply.go     # ordered apply logic + result capture
├── reconcile_keys.go      # normalization/key helpers
└── reconcile_errors.go    # typed ambiguity/safety errors
```

### Pattern 1: Three-Set Reconcile With Ownership Gate
**What:** Build desired/visible/owned indexes first, then plan create/update/delete; require owned-state membership for update/delete.
**When to use:** Every reconcile cycle in this phase.
**Example:**
```go
// Source: internal contracts + state model in repo
for key, owned := range ownedByKey {
    desired, want := desiredByKey[key]
    visible, hasVisible := visibleByKey[key]

    switch {
    case want && hasVisible:
        planUpdateIfChanged(owned, visible, desired)
    case want && !hasVisible:
        planCreate(desired)
    case !want && hasVisible:
        planDeleteIfOwned(owned, visible)
    default:
        // stale ownership pointer; keep conservative and report
    }
}
```

### Pattern 2: Apply-Then-Persist Snapshot
**What:** Execute remote mutations first; write state only from successful operations and current observed facts.
**When to use:** End of each successful reconcile cycle.
**Example:**
```go
// Source: internal/state/store.go
if applyErr == nil {
    next.ManagedRecords = appliedRecords
    if err := store.Save(next); err != nil {
        return fmt.Errorf("persist ownership snapshot: %w", err)
    }
}
```

### Anti-Patterns to Avoid
- **Provider-owned reconciliation:** pushes safety policy into transport adapters and breaks D-05 centralization.
- **Preemptive state writes:** persisting before remote success can mark non-existent ownership and cause unsafe future deletes.
- **Best-effort destructive cleanup of unknown records:** violates RECON-04 by touching operator-managed rewrites.
- **Ignoring duplicate visible keys:** can target the wrong remote record under ambiguous state.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Atomic persistence | Custom partial-write logic | Existing `internal/state` + `atomicWriteFile` | Already handles replace semantics and file mode. [VERIFIED: codebase grep] |
| API contract discovery | Guessing AdGuard payload shapes | Official OpenAPI schema | Avoids drift and request-shape bugs. [CITED: https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/openapi/openapi.yaml] |
| Ownership policy location | Ad-hoc per-provider checks | Single runtime reconcile policy | Keeps safety logic testable and uniform. |

**Key insight:** In this phase, correctness depends more on policy placement and ordering than on algorithmic novelty.

## Common Pitfalls

### Pitfall 1: Accidental mutation of manual records
**What goes wrong:** Deletes/updates are issued from visible diff alone.
**Why it happens:** Owned-state membership is not used as the destructive guard.
**How to avoid:** Require matching `state.ManagedRecord` before any delete/update operation.
**Warning signs:** Reconcile plan contains delete/update for keys absent from local state.

### Pitfall 2: Ambiguous duplicate remote entries
**What goes wrong:** Reconciler mutates one of multiple identical visible entries nondeterministically.
**Why it happens:** Assuming `(domain, answer)` is globally unique in remote state.
**How to avoid:** Detect duplicates while indexing visible records; surface warning/error and skip destructive mutation for that key.
**Warning signs:** Visible index sees `len(recordsByKey[key]) > 1`.

### Pitfall 3: State drift from wrong ordering
**What goes wrong:** State says record exists/changed when remote call failed.
**Why it happens:** State persisted before apply completion.
**How to avoid:** Persist only after successful apply set; derive `LastAppliedAt` from actual successful operation results.
**Warning signs:** Failed apply followed by unchanged remote but advanced `LastAppliedAt`.

## Code Examples

### AdGuard rewrite endpoints used by this phase
```http
GET  /control/rewrite/list
POST /control/rewrite/add
POST /control/rewrite/delete
PUT  /control/rewrite/update
```
Source: [CITED: https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/openapi/openapi.yaml]

### Existing ownership record shape (traceability target)
```go
type ManagedRecord struct {
    Output        contracts.ProviderRef
    Source        contracts.SourceObjectRef
    Hostname      string
    Answer        string
    LastAppliedAt time.Time
}
```
Source: [VERIFIED: codebase grep]

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Rewrite add/delete only | Add + delete + update item APIs available | `v0.107.30` introduced update endpoint | Enables idempotent update flows without delete+recreate churn. [CITED: https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/openapi/CHANGELOG.md] |
| Limited rewrite controls | Rewrite settings endpoints added (`/rewrite/settings`) | `v0.107.68` | Future-proofing for global rewrite toggles; not needed for Phase 02 core diffing. [CITED: https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/openapi/CHANGELOG.md] |

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | A deterministic sort order for planned operations (create/update/delete ordering) should be enforced for stable tests/logs | Architecture Patterns | Low (test flakiness / noisy diffs) |

## Open Questions (RESOLVED)

1. **How should duplicate-key ambiguity be surfaced at runtime?**
   - Resolved for Phase 02: implement a typed ambiguity result plus a warning log, and keep the affected key non-destructive by default.
   - Follow-up: defer any strict fail-fast toggle to a later ops-focused phase.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | Build/test/reconcile implementation | ✓ | go1.26.3 | — |
| Docker CLI/Engine access | Future integration tests (Phase 03/04 mostly) | ✓ | 29.4.1 | Unit tests only |
| AdGuard Home instance | True end-to-end reconcile verification | ✗ (not detected in this session) | — | Stub/mocked output tests in this phase |
| Context7 CLI (`ctx7`) | Preferred doc lookup fallback path | ✗ | — | Official docs via WebFetch |

**Missing dependencies with no fallback:**
- None blocking for Phase 02 implementation planning.

**Missing dependencies with fallback:**
- Live AdGuard instance (fallback: contract-level tests and provider stubs).

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` (stdlib) [VERIFIED: codebase grep] |
| Config file | none (stdlib conventions) [VERIFIED: codebase grep] |
| Quick run command | `mise exec -- go test ./internal/runtime -run Reconcile -count=1` |
| Full suite command | `mise exec -- go test ./... -count=1` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| RECON-02 | idempotent create/update/delete planning + apply | unit | `go test ./internal/runtime -run ReconcilePlanApply -count=1` | ❌ Wave 0 |
| RECON-03 | destructive ops gated by owned state | unit | `go test ./internal/runtime -run OwnershipBoundary -count=1` | ❌ Wave 0 |
| RECON-04 | unmanaged visible records preserved | unit | `go test ./internal/runtime -run PreserveManualRecords -count=1` | ❌ Wave 0 |
| STATE-02 | state snapshot traceability and apply-then-save | unit | `go test ./internal/runtime -run PersistedManagedRecords -count=1` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/runtime -count=1`
- **Per wave merge:** `go test ./... -count=1`
- **Phase gate:** Full suite green before `/gsd-verify-work`

### Wave 0 Gaps
- [ ] `internal/runtime/reconcile_test.go` — core diff/apply/ownership tests (REQ: RECON-02/03/04)
- [ ] `internal/runtime/reconcile_state_test.go` — state ordering and LastAppliedAt tests (REQ: STATE-02)
- [ ] `internal/providers/adguard/output_test.go` or equivalent — payload contract tests for rewrite endpoints

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes | Use AdGuard API auth configured via explicit username + secret resolution (`ENV:` refs). [VERIFIED: codebase grep] |
| V3 Session Management | no | Not a browser-session application in this phase. |
| V4 Access Control | yes | Ownership-boundary authorization: destructive mutation only for locally owned records. |
| V5 Input Validation | yes | Normalize and validate hostname/answer keys before planning operations. |
| V6 Cryptography | yes | Do not hand-roll; rely on Go stdlib TLS/HTTP primitives and secret handling. |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Unauthorized mutation of manual DNS records | Tampering | Enforce D-01 ownership gate before update/delete. |
| Secret leakage through logs/errors | Information Disclosure | Keep password values out of logs; use resolved secret flow already tested in config package. [VERIFIED: codebase grep] |
| Replay/partial failure state corruption | Tampering | Apply remote changes first; atomic persist only after success. |

## Sources

### Primary (HIGH confidence)
- Repository source files (`internal/contracts/*.go`, `internal/runtime/app.go`, `internal/state/*.go`, tests) — architecture/contracts/state behavior [VERIFIED: codebase grep]
- AdGuard OpenAPI spec — rewrite endpoints and payload schemas: https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/openapi/openapi.yaml
- AdGuard technical doc — rewrite list/add/delete semantics: https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/AGHTechDoc.md
- AdGuard API changelog — rewrite update/settings timeline: https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/openapi/CHANGELOG.md

### Secondary (MEDIUM confidence)
- Docker SDK examples page for Moby client usage context: https://docs.docker.com/reference/api/engine/sdk/examples/

### Tertiary (LOW confidence)
- None

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - existing repo stack and official AdGuard API docs align directly.
- Architecture: HIGH - locked decisions (D-01..D-05) are explicit and match current contracts.
- Pitfalls: HIGH - directly derived from requirement safety boundaries plus documented API shape.

**Research date:** 2026-05-13
**Valid until:** 2026-06-12
