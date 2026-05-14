# Phase 3: Docker/Godoxy Snapshot Automation - Context

**Gathered:** 2026-05-13 (assumptions mode)
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 3 establishes Docker/Godoxy-driven desired-record snapshot generation for startup reconciliation. It locks the MVP label-compatibility subset, record derivation rules, and how Docker source output feeds the existing reconcile path, without pulling Docker event watching, reconnect handling, retry orchestration, or deployment hardening into scope.
</domain>

<decisions>
## Implementation Decisions

### Startup Snapshot Boundary
- **D-01:** Phase 3 must implement Docker/Godoxy desired-state derivation through the existing `contracts.Source.ListDesired(context.Context)` snapshot path and feed the already-shipped startup reconcile flow.
- **D-02:** Phase 3 must not add Docker event watching or reconnect behavior; those remain Phase 4 work.

### Source Identity And Desired Records
- **D-03:** Docker container ID remains the stable `Source.ID` for Docker-derived desired records.
- **D-04:** Each derived hostname or alias becomes its own `contracts.DesiredRecord`, while all records from the same container share that container lineage through `DesiredRecord.Source`.

### Godoxy Compatibility Subset
- **D-05:** Freeze MVP support to the DNS-relevant Godoxy label subset only: `proxy.aliases`, `proxy.exclude`, fallback to container name when aliases are absent, and the alias/port forms needed for common Godoxy alias discovery (`proxy.<alias>.port`, `proxy.#N.port`, and `proxy.*.port`).
- **D-06:** Treat broader Godoxy route configuration such as middleware labels, homepage labels, and YAML blob labels as out of scope for Phase 3.

### Parsing And Target Resolution
- **D-07:** Mirror a documented, test-backed subset of Godoxy behavior instead of importing or reusing Godoxy's full parser stack.
- **D-08:** Keep the Docker source config surface endpoint-only unless planning proves a concrete gap; Phase 3 should derive behavior from Docker metadata and supported labels rather than adding new operator knobs by default.
- **D-09:** Resolve each rewrite `answer` using Godoxy-style host-target precedence: use an explicit supported host override when present, otherwise use the derived non-local endpoint host target and never fall back to container IPs.

### the agent's Discretion
- Exact internal package layout for Docker label parsing and hostname derivation, as long as runtime still owns startup orchestration and the source contract stays snapshot-based.
- Whether alias expansion helpers live inside `internal/providers/docker` or a nearby internal package, as long as they stay scoped to the supported Phase 3 subset.
- Exact test fixture organization, provided the locked subset above is covered by deterministic compatibility tests.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase Scope
- `.planning/ROADMAP.md` - Phase 3 goal, requirements mapping, and success criteria.
- `.planning/REQUIREMENTS.md` - `SRC-01`, `SRC-02`, `SRC-03`, and `RECON-01` acceptance constraints for this phase.
- `.planning/STATE.md` - Current blocker calling for the MVP Godoxy compatibility subset to be frozen before planning.

### Product And Prior-Phase Constraints
- `.planning/PROJECT.md` - Product boundaries, safety constraints, and the decision to derive desired state from Docker labels.
- `PRD.md` - Docker/Godoxy source rules, startup reconciliation expectations, and the remaining label-target open questions resolved here.
- `.planning/phases/01-runtime-foundation-contracts/01-CONTEXT.md` - Locked runtime, source-contract, and Docker source identity decisions.
- `.planning/phases/02-ownership-safe-reconciliation-core/02-CONTEXT.md` - Locked reconcile placement and ownership-safe mutation decisions that Phase 3 must feed without changing.

### Existing Implementation Seams
- `internal/contracts/source.go` - Current source contract and `DesiredRecord` shape.
- `internal/runtime/app.go` - Existing startup flow that aggregates `ListDesired` output and reconciles it.
- `internal/runtime/app_test.go` - Existing tests proving startup reconcile runs before steady-state cancellation.
- `internal/runtime/reconcile_plan.go` - Ownership lineage and desired-record diff behavior Phase 3 must remain compatible with.
- `internal/state/model.go` - Persisted managed-record shape that consumes Docker source identity and desired hostname/answer pairs.
- `internal/config/model.go` - Current Docker source config surface.
- `internal/config/validate.go` - Current validation constraints for Docker source configuration.
- `internal/providers/docker/source.go` - Current Docker provider construction and the unimplemented `ListDesired` seam.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/providers/docker/source.go` already owns Docker client construction with API-version negotiation and is the natural place to add snapshot listing from container metadata.
- `internal/contracts/source.go` already provides the exact output shape Phase 3 needs: a source-owned list of normalized `DesiredRecord` values.
- `internal/runtime/app.go` already performs startup full reconciliation by aggregating all source snapshots before diffing against output-visible and owned state.
- `internal/runtime/app_test.go` already proves startup reconciliation happens before the runtime blocks on steady state, so Phase 3 can extend existing startup behavior instead of inventing a second path.
- `internal/runtime/reconcile_plan.go` and `internal/state/model.go` already preserve per-container lineage through `Source.ID`, `Hostname`, and `Answer`, which supports one-container-many-hostnames derivation cleanly.

### Established Patterns
- Runtime owns orchestration, provider construction, and reconciliation; providers stay narrow and policy-free.
- Source implementations are expected to translate provider-native metadata into normalized desired records rather than leaking provider-specific structures upward.
- The current config model is explicit and minimal, so new behavior should default to metadata-driven derivation before adding Docker-specific config knobs.
- The existing reconcile engine assumes stable source lineage and deterministic desired hostname/answer pairs, so Docker derivation should optimize for reproducible snapshots.

### Integration Points
- The Docker provider's `ListDesired` implementation will plug directly into `App.startupReconcile` without changing runtime contracts.
- Derived `DesiredRecord.Source` values must align with the lineage keying in `internal/runtime/reconcile_plan.go` so alias additions, removals, and target changes reconcile safely.
- Any supported host-override semantics must emit plain `Hostname` and `Answer` values that the existing AdGuard output and state model can persist unchanged.
</code_context>

<specifics>
## Specific Ideas

- Validate the locked Godoxy subset against upstream docs/tests during planning and implementation, but keep support intentionally narrower than the full Godoxy route-label surface.
- Use upstream Godoxy behavior as a semantic reference for alias expansion, exclusion, and host-target precedence without coupling this daemon to Godoxy's broader route parser internals.
</specifics>

<deferred>
## Deferred Ideas

- Docker event watching, event-triggered re-listing, reconnect handling, and missed-event recovery - Phase 4.
- AdGuard outage retries and broader runtime recovery loops - Phase 4.
- Middleware/homepage/YAML-based Godoxy route configuration support - future compatibility expansion, not required for MVP Phase 3.
- Richer Docker-source config knobs beyond endpoint selection - only add later if the frozen MVP subset proves a concrete need.

### Reviewed Todos (not folded)
None - no matching todos were present for Phase 3.
</deferred>
