---
phase: 1
slug: runtime-foundation-contracts
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-05-12
---

# Phase 1 - Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` package |
| **Config file** | none |
| **Quick run command** | `go test ./internal/config ./internal/runtime ./internal/state ./cmd/docker-dns-sync` |
| **Full suite command** | `go test ./...` |
| **Estimated runtime** | ~30 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/config ./internal/runtime ./internal/state ./cmd/docker-dns-sync`
- **After every plan wave:** Run `go test ./...`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 1-01-01 | 01 | 1 | CONF-01 | T-01-01 / T-01-02 | Startup scaffolding and fixtures lock the Phase 1 config contract | structural | `test -f cmd/docker-dns-sync/main_test.go && test -f testdata/config/minimal.toml && test -f testdata/config/malformed.toml` | ❌ W0 | ⬜ pending |
| 1-01-02 | 01 | 1 | CONF-01 | T-01-01 / T-01-02 | Daemon startup accepts valid config and stays alive until cancelled | unit | `mise exec -- go test ./cmd/docker-dns-sync ./internal/config ./internal/runtime -run 'TestRunWithMinimalConfig|TestRunRejectsMissingConfig|TestRunRejectsMalformedConfig|TestRunBlocksUntilCancelled' -count=1` | ❌ W0 | ⬜ pending |
| 1-02-01 | 02 | 2 | CONF-01, CONF-02, CONF-03 | T-01-04 / T-01-05 | Config-validation scaffolding and fixtures exist for runtime and secret rules | structural | `test -f internal/config/validate_test.go && test -f testdata/config/socket-proxy.toml && test -f testdata/config/env-secret.toml` | ❌ W0 | ⬜ pending |
| 1-02-02 | 02 | 2 | CONF-01, CONF-02, CONF-03 | T-01-04 / T-01-05 | Semantic validation rejects unusable config and ambiguous secrets | unit | `mise exec -- go test ./internal/config -run 'TestValidateRequiresSourceAndOutput|TestValidateRuntimeAndCredentialFields|TestDockerEndpointModes|TestResolveSecrets' -count=1` | ❌ W0 | ⬜ pending |
| 1-03-01 | 03 | 3 | OPS-03, CONF-03 | T-01-07 / T-01-08 | Contract and factory test scaffolding exists for provider extensibility | structural | `test -f internal/contracts/source.go && test -f internal/contracts/output.go && test -f internal/runtime/factories_test.go` | ❌ W0 | ⬜ pending |
| 1-03-02 | 03 | 3 | OPS-03, CONF-03 | T-01-07 / T-01-08 | Runtime can build providers through factories and honor Docker endpoint config | unit | `mise exec -- go test ./internal/runtime -run 'TestFactoryRegistryExtensibility|TestBuildProvidersFromConfig|TestDockerSourceUsesConfiguredEndpoint' -count=1` | ❌ W0 | ⬜ pending |
| 1-04-01 | 04 | 4 | CONF-02 | T-01-10 / T-01-12 | State and final runtime test scaffolding exists for the fully wired daemon path | structural | `test -f internal/state/atomic_file_test.go && test -f cmd/docker-dns-sync/main_test.go` | ❌ W0 | ⬜ pending |
| 1-04-02 | 04 | 4 | CONF-02 | T-01-10 / T-01-12 | Runtime applies configured log level and retry policy while initializing state safely | unit | `mise exec -- go test ./internal/state ./internal/runtime ./cmd/docker-dns-sync -run 'TestAtomicWriteReplacesStateFile|TestRunInitializesStateStoreAndProviders|TestRuntimeAppliesLoggingAndRetryConfig|TestRunBlocksUntilCancelled' -count=1` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠ flaky*

---

## Wave 0 Requirements

- [ ] `cmd/docker-dns-sync/main_test.go` - startup smoke and fully wired runtime tests
- [ ] `internal/config/validate_test.go` - config and secret contract coverage
- [ ] `internal/runtime/factories_test.go` - provider extensibility and Docker endpoint bootstrap coverage
- [ ] `internal/state/atomic_file_test.go` - atomic state persistence coverage

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| `timeout 5s mise exec -- go run ./cmd/docker-dns-sync -config testdata/config/minimal.toml` stays running until terminated with stub output wiring | CONF-01 | Final operator-facing smoke pass is simpler as a direct command check than a unit assertion | Run the command under `mise`, verify it stays alive until timeout/termination, and confirm no panic or secret output |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 30s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
