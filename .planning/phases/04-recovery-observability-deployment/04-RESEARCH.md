# Phase 04: recovery-observability-deployment - Research

**Researched:** 2026-05-13
**Domain:** Runtime recovery, retry-aware reconciliation, structured observability, and deployment hardening for a Go infrastructure daemon
**Confidence:** HIGH

## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Reuse startup reconciliation as the durable correctness path and add steady-state orchestration around it.
- **D-02:** Keep snapshot listing mandatory and make live watch behavior optional.
- **D-03:** Treat Docker events as hints, not direct state mutations.
- **D-04:** Reconcile after Docker watch disconnect before trusting resumed event handling.
- **D-05:** Keep retry and failure handling outside provider-owned safety policy.
- **D-06:** Emit structured retry/recovery logs without exposing secrets.
- **D-09:** Ship first-class host-binary and Docker deployment paths in-repo.

### the agent's Discretion
- Exact watch-hint payload shape and whether runtime reacts with global or source-scoped re-listing.
- Exact retry helper placement and log field schema.
- Exact deployment file layout and example naming.

### Deferred Ideas (OUT OF SCOPE)
- Metrics endpoints, health APIs, and Prometheus support.
- Dry-run/inspect mode and richer drift diagnostics.
- Additional sources and outputs.

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| STATE-01 | Restart/reboot restores correct managed rewrite state | Startup reconcile remains the restore path; tests should prove persisted state + source snapshot convergence |
| STATE-03 | Docker event stream disconnect recovery | Optional watch contract, reconnect loop, and post-disconnect reconcile |
| STATE-04 | Temporary AdGuard outage recovery | Retry-aware visible/apply operations with bounded backoff and preserved state ordering |
| OPS-01 | Structured logs for runtime operations | Consistent log points in runtime loop, retries, apply steps, and persistence |
| OPS-02 | First-class host-binary and Docker deployment | `systemd` unit, container image, example config/docs, explicit socket/state mounts |

## Summary

The current runtime already has the key durability primitive for restart and reboot recovery: startup loads persisted state, lists desired source state, lists visible output state, reconciles, and persists the next snapshot. That means Phase 4 does not need a second recovery algorithm for restart correctness; it needs to preserve this path and make it re-entrant after live runtime failures. [VERIFIED: codebase grep]

Docker event streaming in the Moby client is explicitly caller-managed: the stream ends on error and the caller must reopen it. That makes a reconnect loop in `internal/runtime` or the Docker source's watch seam the right place to implement disconnect recovery. Events should be treated as reconcile triggers, not direct record deltas, because Phase 3 intentionally froze Docker behavior as snapshot-derived desired state. [CITED: https://github.com/moby/moby/blob/client/v0.4.0/client/system_events.go] [VERIFIED: codebase grep]

AdGuard rewrite operations are still item-level list/add/update/delete calls with no daemon metadata or transaction boundary. Temporary outages therefore need retry-aware list/apply behavior that keeps the existing state-write ordering intact: retry remote operations, but never persist ownership state before the remote side succeeds. [CITED: https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/AGHTechDoc.md] [VERIFIED: codebase grep]

For deployment, the repo currently lacks a Dockerfile, service unit, or operator-facing setup docs. `systemd` service docs support the expected daemon controls (`ExecStart`, `Restart`, `RestartSec`), and Docker's own daemon-access guidance reinforces that socket access is privileged and should be explicit in operator docs. [CITED: https://www.freedesktop.org/software/systemd/man/254/systemd.service.html] [CITED: https://docs.docker.com/engine/security/protect-access/]

**Primary recommendation:** Split Phase 4 into three plans: add the watch/reconnect runtime loop, add retry-aware output and structured logs, and then ship deployment artifacts plus operator documentation.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Startup + steady-state reconcile orchestration | API / Backend | — | Runtime already owns correctness policy and should stay the control point. |
| Docker event watch and reconnect | API / Backend | External Docker daemon | Events are upstream hints and need caller-managed reconnect semantics. |
| AdGuard outage retries | API / Backend | External AdGuard Home | Retries must preserve runtime state ordering and keep provider logic transport-focused. |
| Structured runtime logging | API / Backend | — | Logs are emitted around orchestration boundaries, not just transport calls. |
| Deployment artifacts | Operations | API / Backend | Service/container packaging must reflect actual runtime CLI and file/socket needs. |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go toolchain | 1.26.3 | Runtime/compiler for daemon | Already pinned in `mise.toml`. [VERIFIED: codebase grep] |
| Go stdlib (`context`, `time`, `log/slog`, `errors`) | go1.26.3 | Runtime loop, backoff timing, structured logs | Enough for this phase without a new dependency. [VERIFIED: codebase grep] |
| `github.com/moby/moby/client` | v0.4.1 | Docker snapshot + event streaming | Existing provider already uses it. [VERIFIED: codebase grep] |
| `internal/runtime`, `internal/contracts`, `internal/state` | current repo | Orchestration, contracts, atomic persistence | Existing architecture already centralizes safety policy. [VERIFIED: codebase grep] |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `log/slog` | stdlib | Structured operator logs | Reuse current logger dependency instead of adding a logging package. |
| Existing retry config in `config.RetryConfig` | current repo | Backoff shape for reconnects/outages | Reuse already-validated configuration instead of adding new knobs. |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Optional watch interface adjacent to `Source` | Add `Watch` directly to `Source` | Forces every source to implement live watch and breaks current narrow startup-only contract. |
| Snapshot reconcile after event hints | Translate Docker events directly into create/update/delete intents | Faster in theory, but duplicates source derivation logic and increases missed-event risk. |
| Runtime/backoff helpers | Provider-internal retries | Hides retry semantics from orchestration logs and risks mixing safety policy with transport behavior. |
| `systemd` + Docker examples | Only a README paragraph | Too weak for OPS-02 first-class deployment support. |

**Installation:**
```bash
# No new external dependencies required for Phase 04 planning.
```

## Package Legitimacy Audit

No new external package is required for the recommended Phase 4 design. Existing stdlib timing/logging plus the already-adopted Moby client are sufficient. [VERIFIED: codebase grep]

## Architecture Patterns

### System Architecture Diagram

```text
Startup:
  load state -> list desired -> list visible -> reconcile -> persist

Steady state:
  watch source hints -> rerun snapshot reconcile
                 \-> on watch error: backoff -> reconnect -> reconcile -> resume watch

Output failures:
  visible/apply call fails -> retry with backoff -> on success continue
                           -> on terminal failure return error without state advance
```

### Pattern 1: Optional Watch Capability
**What:** Keep `ListDesired` required and gate live behavior behind a second interface checked with type assertion.
**When to use:** Runtime steady-state loop for Docker and future sources.
**Example:**
```go
type WatchableSource interface {
    Watch(context.Context) (<-chan struct{}, <-chan error)
}

if watcher, ok := source.(WatchableSource); ok {
    // start steady-state loop
}
```

### Pattern 2: Reconcile-After-Hint
**What:** On each event hint, re-list desired state and visible state, then reuse the same reconcile entrypoint.
**When to use:** Docker container start/stop/destroy/update events and reconnect recovery.
**Example:**
```go
func (a *App) reconcileAll(ctx context.Context) error {
    owned, _ := a.store.Load()
    desired := collectDesired(ctx, a.sources)
    for _, output := range a.outputs {
        visible, _ := output.ListVisible(ctx)
        owned, _ = ReconcileAndPersist(ctx, a.store, ReconcileInput{...}).Next
    }
    return nil
}
```

### Pattern 3: Retry Without State Advance
**What:** Retry remote reads/writes with backoff, but only persist state after a successful reconcile apply.
**When to use:** Temporary AdGuard outages and Docker watch reconnect loops.
**Example:**
```go
for attempt := 1; ; attempt++ {
    err := output.Create(ctx, desired)
    if err == nil {
        break
    }
    if !retryable(err) || deadlineExceeded(...) {
        return err
    }
    logger.Warn("retrying output operation", "attempt", attempt, "err", err)
    sleepBackoff(...)
}
```

### Anti-Patterns to Avoid
- **Event-as-truth logic:** deriving record diffs directly from Docker events instead of reusing snapshot derivation.
- **Provider-owned reconcile retries with hidden sleeps:** makes failure behavior hard to reason about and test.
- **Success logs without scope:** operator logs need source/output identity and operation type, not generic messages.
- **Deployment docs without mounts/permissions:** leaves operators unable to persist state or access Docker safely.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Event transport protocol parsing | Custom Docker socket event reader | Existing Moby client `Events` support | Already matches current provider stack. [CITED: https://github.com/moby/moby/blob/client/v0.4.0/client/system_events.go] |
| Logging framework swap | New logging dependency | Existing `log/slog` runtime logger | Already configured and sufficient. [VERIFIED: codebase grep] |
| Service supervision semantics | Custom shell restart loop | `systemd` `Restart=` / Docker restart policy docs | Better operator fit and less bespoke behavior. [CITED: https://www.freedesktop.org/software/systemd/man/254/systemd.service.html] |

## Common Pitfalls

### Pitfall 1: Missing events during reconnect
**What goes wrong:** The daemon resumes watching after a disconnect without reconciling the missed window.
**How to avoid:** Run a full reconcile after reconnect and before resuming normal event trust.

### Pitfall 2: Retrying after state already changed
**What goes wrong:** A failed remote call leaves local state ahead of AdGuard.
**How to avoid:** Keep retries before `store.Save` and never persist speculative ownership.

### Pitfall 3: Deployment examples that do not persist state
**What goes wrong:** Container restart loses ownership snapshot and causes avoidable churn.
**How to avoid:** Document and ship a mounted state path in Docker examples and writable path in `systemd` examples.

## Key Insight

Phase 4 is mostly an orchestration hardening phase: the core reconcile and ownership model already exist, so the work is to make that model run continuously, recover predictably, and ship in forms operators can actually use.
