---
phase: 3
slug: docker-godoxy-snapshot-automation
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-05-13
---

# Phase 3 - Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` package |
| **Config file** | none |
| **Quick run command** | `mise exec -- go test ./internal/providers/docker ./internal/runtime -count=1` |
| **Full suite command** | `mise exec -- go test ./... -count=1` |
| **Estimated runtime** | ~45 seconds |

---

## Sampling Rate

- **After every task commit:** Run `mise exec -- go test ./internal/providers/docker ./internal/runtime -count=1`
- **After every plan wave:** Run `mise exec -- go test ./... -count=1`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 45 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 3-01-01 | 01 | 1 | SRC-01, SRC-02, SRC-03 | T-03-01 / T-03-02 | Subset label compatibility scaffolding exists for alias fallback, exclusion, wildcard/reference ports, host-target precedence, and preserved `#N` alias positions | structural | `test -f internal/providers/docker/labels_test.go && test -f internal/providers/docker/source_test.go` | ❌ W0 | ⬜ pending |
| 3-01-02 | 01 | 1 | SRC-01, SRC-02, SRC-03 | T-03-01 / T-03-02 | Docker label parsing emits deterministic desired records only for supported and eligible containers, keeps indexed host overrides bound to the original alias slot, uses the non-local endpoint host as the default target, and omits local records that lack an explicit host target | unit | `mise exec -- go test ./internal/providers/docker -run 'DockerLabels|DesiredRecordDerivation' -count=1` | ❌ W0 | ⬜ pending |
| 3-02-01 | 02 | 1 | SRC-01, RECON-01 | T-03-03 / T-03-04 | Docker source snapshot scaffolding exists and runtime startup coverage still references the real source seam | structural | `test -f internal/providers/docker/source.go && test -f internal/runtime/app_test.go` | ❌ W0 | ⬜ pending |
| 3-02-02 | 02 | 1 | SRC-01, SRC-02, SRC-03, RECON-01 | T-03-03 / T-03-04 | `ListDesired` lists current containers, filters ineligible/excluded cases, derives deterministic desired records, and feeds startup reconcile | unit | `mise exec -- go test ./internal/providers/docker ./internal/runtime -run 'ListDesired|StartupReconcile' -count=1` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠ flaky*

---

## Wave 0 Requirements

- [ ] `internal/providers/docker/labels_test.go` - locked subset compatibility coverage, including explicit host overrides without localhost or container-IP fallback
- [ ] `internal/providers/docker/source_test.go` - Docker snapshot desired-record coverage
- [ ] `internal/runtime/app_test.go` - startup reconcile integration still proven

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Desired records derived from a real local Docker daemon match expected aliases and exclusions for one representative labeled container | SRC-01, SRC-02, SRC-03, RECON-01 | Requires a live Docker environment and operator-visible fixture setup not guaranteed in unit tests | Run the daemon against a local Docker host with one excluded container and one multi-alias container, confirm startup reconcile creates only the expected rewrites and persisted state entries |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 45s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
