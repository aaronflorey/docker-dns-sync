---
phase: 4
slug: recovery-observability-deployment
status: complete
nyquist_compliant: true
wave_0_complete: true
created: 2026-05-13
---

# Phase 4 - Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` package |
| **Config file** | none yet; sample artifacts added in Plan 04-03 |
| **Quick run command** | `mise exec -- go test ./internal/runtime ./cmd/docker-dns-sync -count=1` |
| **Full suite command** | `mise exec -- go test ./... -count=1` |
| **Race suite command** | `mise exec -- go test -race ./internal/runtime ./internal/providers/docker -count=1` |
| **Estimated runtime** | ~60 seconds |

---

## Sampling Rate

- **After every task commit:** Run the narrow package tests touched by the task.
- **After every plan wave:** Run `mise exec -- go test ./... -count=1`.
- **Before `/gsd-verify-work`:** Full suite must be green and deployment artifacts must exist.
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 4-01-01 | 01 | 1 | STATE-01, STATE-03 | T-04-01 / T-04-02 | Optional watch contract and runtime-loop tests exist for steady-state reconcile, restart, and reconnect paths | structural | `test -f internal/runtime/app_test.go && test -f internal/providers/docker/source_test.go && test -f internal/contracts/source.go` | ✅ | ✅ green |
| 4-01-02 | 01 | 1 | STATE-01, STATE-03 | T-04-01 / T-04-02 | Runtime reacts only to relevant Docker hints, keeps per-source reconnect backoff from blocking unrelated steady-state watch hints, runs a post-watch startup handoff resync through the watch retry path, treats clean watch closure as disconnect, reconnects after stream errors with backoff, resets reconnect backoff after successful repair/handoff, and retries reconnect repair when Docker is briefly unavailable while ignoring unrelated network connect/disconnect activity | unit | `mise exec -- go test ./internal/runtime ./internal/providers/docker -run 'Watch|Reconnect|StartupReconcile|RuntimeReconcile|CleanWatchClosure|Resync|BacksOffRepeatedWatchReconnects|ResetsReconnectBackoffAfterSuccessfulRepair|ShouldTrigger(Container|Network)WatchHint|DoesNotBlockOtherWatchHints' -count=1 && mise exec -- go test -race ./internal/runtime ./internal/providers/docker -count=1` | ✅ | ✅ green |
| 4-02-01 | 02 | 1 | STATE-04, OPS-01 | T-04-03 / T-04-04 | Retry/logging scaffolding exists for full-reconcile output read/write retries, single-shot per-attempt mutations, and runtime recovery logs | structural | `test -f internal/runtime/deps.go && test -f internal/runtime/app_test.go && test -f internal/providers/adguard/output_test.go && test -f cmd/docker-dns-sync/main_test.go` | ✅ | ✅ green |
| 4-02-02 | 02 | 1 | STATE-04, OPS-01 | T-04-03 / T-04-04 | Temporary output read failures retry with bounded backoff from the reconcile boundary, temporary AdGuard write failures rerun a bounded full reconcile instead of inline RPC retries, watch-triggered full reconciles including the startup handoff retry transient source-read failures from any configured source, partial mutation progress is persisted safely, and logs show attempts without secrets | unit | `mise exec -- go test ./internal/runtime ./internal/providers/adguard ./cmd/docker-dns-sync -run 'Retry|Logging|Runtime|PersistedManagedRecords|Temporary|Terminal' -count=1 && mise exec -- go test -race ./internal/runtime ./internal/providers/docker -count=1` | ✅ | ✅ green |
| 4-03-01 | 03 | 1 | OPS-02 | T-04-05 | Deployment artifacts and docs exist for host-binary and Docker modes, including local-socket and proxy/remote example configs | structural | `test -f README.md && test -f deploy/systemd/docker-dns-sync.service && test -f Dockerfile && test -f testdata/config/example.toml && test -f testdata/config/socket-proxy.toml` | ✅ | ✅ green |
| 4-03-02 | 03 | 1 | OPS-02 | T-04-05 | Docs and examples invoke the real CLI/config contract, call out Docker socket permissions explicitly, and cover both local-socket and proxy/remote deployment paths | structural | `grep -nE '\-config|docker.sock|socket-proxy|tcp://' README.md deploy/systemd/docker-dns-sync.service Dockerfile testdata/config/example.toml testdata/config/socket-proxy.toml` | ✅ | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠ flaky*

---

## Wave 0 Requirements

- [x] `internal/runtime/app_test.go` - steady-state watch/reconnect/retry orchestration coverage, including startup handoff resync retries, non-blocking per-source reconnect backoff, clean closure recovery, reconnect backoff reset after successful repair, and transient source failures across watch-triggered full reconciles
- [x] `internal/providers/docker/source_test.go` - Docker watch seam coverage, including container-scoped network connect/disconnect hints and irrelevant container-event filtering
- [x] `internal/providers/adguard/output_test.go` - temporary-vs-terminal retry classification and sanitized error-surface coverage
- [x] `README.md` - operator deployment instructions
- [x] `deploy/systemd/docker-dns-sync.service` - host-binary service artifact
- [x] `Dockerfile` - container deployment artifact
- [x] `testdata/config/socket-proxy.toml` - proxy/remote Docker deployment example artifact

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Host-binary deployment can access Docker and persist state with expected file ownership | OPS-02 | Depends on local host permissions, service manager, and operator environment | Install the sample `systemd` unit with a real config, grant the service user access to either `/var/run/docker.sock` or the configured remote/proxy endpoint, start the service, verify state file creation, restart the service, and confirm managed rewrites reconcile back without manual cleanup |
| Docker deployment can run with mounted socket/proxy and persistent state volume | OPS-02 | Depends on local Docker runtime and mount semantics | Build/run the container artifact with mounted config and state paths, verify the documented local-socket mount or proxy endpoint is reachable from the container, then restart the container and confirm state and reconciliation survive restart |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all missing references
- [x] No watch-mode flags
- [x] Feedback latency < 60s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** verified 2026-05-13 via `test -f README.md && test -f deploy/systemd/docker-dns-sync.service && test -f Dockerfile && test -f testdata/config/example.toml && test -f testdata/config/socket-proxy.toml`, `rg -n '\-config|docker.sock|socket-proxy|tcp://' README.md deploy/systemd/docker-dns-sync.service Dockerfile testdata/config/example.toml testdata/config/socket-proxy.toml`, `mise exec -- go test ./internal/runtime ./internal/providers/adguard ./cmd/docker-dns-sync -run 'Retry|Logging|Runtime|PersistedManagedRecords|Temporary|Terminal' -count=1`, and `mise exec -- go test -race ./internal/runtime ./internal/providers/docker -count=1`
