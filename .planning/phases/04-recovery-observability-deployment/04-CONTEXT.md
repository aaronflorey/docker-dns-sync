# Phase 4: Recovery, Observability & Deployment - Context

**Gathered:** 2026-05-13 (assumptions mode)
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 4 turns the current startup-only daemon into a long-running operator service. It adds Docker event watching, reconnect-driven recovery, retry-aware output behavior, structured recovery logs, and documented deployment artifacts for host-binary and Docker usage.

It must preserve the existing ownership-safe reconcile core and Phase 3 Docker snapshot behavior. This phase should not broaden provider scope beyond Docker/Godoxy input and AdGuard Home output, and it should not add an operator API, UI, metrics endpoint, or dry-run surface.
</domain>

<decisions>
## Implementation Decisions

### Runtime Recovery Boundary
- **D-01:** Keep startup reconciliation as the single source-of-truth entrypoint, then add a steady-state runtime loop that reuses the same snapshot-based reconcile path after change hints and recovery events.
- **D-02:** Preserve `contracts.Source.ListDesired(context.Context)` as the required source contract and introduce any live-watch capability as an optional adjacent interface instead of forcing every source to implement it.
- **D-03:** Treat Docker events as hints to rerun source snapshot logic, not as direct desired-state mutations.
- **D-04:** On Docker event stream error or disconnect, the daemon must reconnect and run a full reconciliation before trusting the stream again.

### Retry And Failure Handling
- **D-05:** Keep ownership and diff policy in `internal/runtime`; retry behavior must not move reconcile safety logic into providers.
- **D-06:** Retry temporary AdGuard output failures with the configured runtime retry policy and surface each retry attempt in structured logs without logging credentials or full secret-bearing config.
- **D-07:** Persist state only after successful reconcile application remains unchanged; retries are for remote visibility/apply operations, not for speculative state writes.

### Observability
- **D-08:** Structured logs must cover startup reconcile, watch-loop start/stop, Docker reconnects, retry attempts, output mutations, state persistence, and terminal error paths.

### Deployment Boundary
- **D-09:** Ship two first-class deployment paths in-repo: host binary under `systemd` and Docker container deployment with explicit socket/state mounts.
- **D-10:** Deployment artifacts should prefer least-privilege defaults already compatible with the current architecture: non-root container image where feasible, explicit state-file mount, and documented Docker socket or proxy access requirements.

### the agent's Discretion
- Exact names and shape of the optional watch interface, as long as startup-only sources remain valid and runtime orchestration stays centralized.
- Whether retry logic is implemented as a runtime helper, provider decorator, or narrow wrapper, as long as provider business logic stays transport-focused.
- Exact deployment artifact layout, as long as both host-binary and Docker paths are documented and testable.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase Scope
- `.planning/ROADMAP.md` - Phase 4 goal, requirements mapping, and success criteria.
- `.planning/REQUIREMENTS.md` - `STATE-01`, `STATE-03`, `STATE-04`, `OPS-01`, and `OPS-02` acceptance constraints.
- `.planning/STATE.md` - Current project position and deployment concern carried forward from prior phases.

### Product And Prior-Phase Constraints
- `.planning/PROJECT.md` - Recovery, safety, and deployment constraints.
- `PRD.md` - Runtime sequence, recovery sequence, structured logging, and documented deployment expectations.
- `.planning/phases/02-ownership-safe-reconciliation-core/02-CONTEXT.md` - Locked ownership and persistence rules.
- `.planning/phases/03-docker-godoxy-snapshot-automation/03-CONTEXT.md` - Locked Docker snapshot and source-lineage rules.

### Existing Implementation Seams
- `internal/runtime/app.go` - Current startup-only orchestration point that Phase 4 must extend without replacing the reconcile core.
- `internal/runtime/reconcile.go` - Existing reconcile-and-persist entrypoint that recovery loops should reuse.
- `internal/runtime/deps.go` - Existing logger and retry-policy dependency carrier.
- `internal/contracts/source.go` - Current required source contract and likely seam for an optional watch capability.
- `internal/providers/docker/source.go` - Current Docker snapshot provider and natural home for Docker event streaming.
- `internal/providers/adguard/output.go` - Current transport-only output provider for visible-state and rewrite CRUD operations.
- `internal/state/store.go` - Current atomic owned-state persistence boundary.
- `cmd/docker-dns-sync/main.go` - Process entrypoint used by host-binary and container deployment artifacts.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/runtime/app.go` already validates config, builds providers, initializes state, logs startup, and runs startup reconciliation before waiting on context cancellation.
- `internal/providers/docker/source.go` already owns the Docker client seam and can widen it to include event streaming without touching config shape.
- `internal/providers/adguard/output.go` already isolates HTTP request construction and error surfaces, which makes it a good candidate for retry-aware wrapping without changing reconcile policy.
- `internal/runtime/app_test.go` already proves startup reconcile ordering and provides a pattern for cancellation-controlled runtime tests.

### Established Patterns
- Runtime owns orchestration and providers stay narrow.
- State writes are atomic and versioned.
- Tests are table-driven and mostly unit-level; current repo has not yet introduced long-running runtime loop tests or deployment artifacts.

### Integration Points
- Docker watch events should feed back into the existing `ListDesired` snapshot path.
- Retry behavior must compose with `ReconcileAndPersist` so failed output writes never advance state.
- Deployment artifacts must invoke the existing CLI with `-config` and must preserve access to the configured state path.
</code_context>

<specifics>
## Specific Ideas

- Use an optional source-watch interface that emits generic change hints or trigger-only notifications so future sources can participate without leaking Docker event types into runtime.
- Prefer full or source-scoped snapshot reruns after events instead of attempting fine-grained event-to-record mutation logic in Phase 4.
- Use runtime-managed backoff for both Docker watch reconnects and temporary AdGuard outages, driven by the existing retry config.
- Add deployment examples that show both direct socket mounting and socket-proxy style endpoint configuration.
</specifics>

<deferred>
## Deferred Ideas

- Metrics endpoint, health endpoint, or Prometheus integration - future `OPX-03`, not Phase 4.
- Dry-run, inspect mode, or operator drift diagnostics - future `OPX-01` / `OPX-02`.
- Additional source or output providers - future `EXT-01` / `EXT-02`.

### Reviewed Todos (not folded)
None - no matching todos were present for Phase 4 beyond the carried deployment parity concern.
</deferred>
