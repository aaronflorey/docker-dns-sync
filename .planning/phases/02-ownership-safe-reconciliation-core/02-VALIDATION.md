---
phase: 2
slug: ownership-safe-reconciliation-core
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-05-13
---

# Phase 2 - Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` package |
| **Config file** | none |
| **Quick run command** | `mise exec -- go test ./internal/runtime ./internal/providers/... ./internal/state -count=1` |
| **Full suite command** | `mise exec -- go test ./... -count=1` |
| **Estimated runtime** | ~45 seconds |

---

## Sampling Rate

- **After every task commit:** Run `mise exec -- go test ./internal/runtime ./internal/providers/... ./internal/state -count=1`
- **After every plan wave:** Run `mise exec -- go test ./... -count=1`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 45 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 2-01-01 | 01 | 1 | RECON-02, RECON-03, RECON-04, STATE-02 | T-02-01 / T-02-03 | Reconcile test scaffolding exists for create, update, delete, ownership-boundary, ambiguity, and persisted-state ordering coverage | structural | `test -f internal/runtime/reconcile_test.go && test -f internal/runtime/reconcile_state_test.go` | ❌ W0 | ⬜ pending |
| 2-01-02 | 01 | 1 | RECON-02, RECON-03, RECON-04, STATE-02 | T-02-01 / T-02-03 | Runtime reconcile planner/apply path safely mutates only owned records, skips ambiguous duplicates, and persists state only after successful apply | unit | `mise exec -- go test ./internal/runtime -run 'ReconcilePlanApply|OwnershipBoundary|PreserveManualRecords|PersistedManagedRecords' -count=1` | ❌ W0 | ⬜ pending |
| 2-02-01 | 02 | 1 | RECON-02, RECON-04 | T-02-04 / T-02-05 | AdGuard provider scaffolding exists with visible-state and item-level rewrite CRUD coverage | structural | `test -f internal/providers/adguard/output.go && test -f internal/providers/adguard/output_test.go && test -f internal/runtime/factories_test.go` | ❌ W0 | ⬜ pending |
| 2-02-02 | 02 | 1 | RECON-02, RECON-04 | T-02-04 / T-02-05 | Runtime-built AdGuard outputs can list visible rewrites and issue item-level add/update/delete requests with the expected request shapes | unit | `mise exec -- go test ./internal/providers/adguard ./internal/runtime -run 'AdGuard|FactoryRegistry' -count=1` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠ flaky*

---

## Wave 0 Requirements

- [ ] `internal/runtime/reconcile_test.go` - reconcile create/update/delete and ownership-boundary coverage
- [ ] `internal/providers/adguard/output_test.go` - AdGuard rewrite endpoint contract coverage
- [ ] `internal/runtime/reconcile_state_test.go` - apply ordering and persistence safety coverage
- [ ] `internal/runtime/factories_test.go` - runtime wiring coverage for the real AdGuard provider

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Reconcile logs clearly explain skipped destructive operations caused by duplicate visible records | RECON-03, RECON-04 | Operator-facing ambiguity diagnostics are easier to review from a real run log than assert exhaustively in unit output | Run the phase reconcile flow against a fixture or local AdGuard instance with duplicate visible rewrites, confirm no delete/update occurs for the ambiguous key, and inspect logs for a clear ambiguity message |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 45s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
