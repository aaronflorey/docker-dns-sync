# Phase 2: Ownership-Safe Reconciliation Core - Context

**Gathered:** 2026-05-12 (assumptions mode)
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 2 establishes the ownership-safe reconciliation core for AdGuard rewrites. It locks how the daemon reads visible AdGuard state, computes safe create/update/delete operations, and persists ownership records without pulling Docker/Godoxy discovery breadth, event-stream recovery, or deployment hardening into scope.
</domain>

<decisions>
## Implementation Decisions

### Ownership Mutation Boundary
- **D-01:** Treat persisted `state.ManagedRecord` entries as the only authority for destructive mutations, so visible AdGuard rewrites not represented in local state are ignored rather than updated or deleted.

### Record Identity For Diffing
- **D-02:** Match managed and visible rewrites by normalized `hostname/domain + answer` within an output provider, because AdGuard does not expose a stable remote record ID for rewrite items.
- **D-03:** Treat duplicate remote entries for the same normalized `hostname/domain + answer` pair as ambiguous state and reconcile conservatively instead of assuming safe targeted mutation.

### State Update Ordering
- **D-04:** Persist ownership state only after successful AdGuard item mutations, and update `LastAppliedAt` from applied results rather than before remote writes.

### Reconciliation Placement And Scope
- **D-05:** Add one centralized reconcile engine in the runtime/core layer that diffs source desired state, visible AdGuard state, and persisted ownership state rather than embedding reconcile behavior inside providers.

### the agent's Discretion
- Exact internal package boundaries for the reconcile engine, as long as reconcile semantics remain centralized and provider contracts stay narrow.
- Whether duplicate remote state is surfaced as structured warnings, typed errors, or both, as long as the behavior is non-destructive by default.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase Scope
- `.planning/ROADMAP.md` - Phase 2 goal, requirements mapping, and success criteria.
- `.planning/REQUIREMENTS.md` - `RECON-02`, `RECON-03`, `RECON-04`, and `STATE-02` acceptance constraints for this phase.
- `.planning/STATE.md` - Current project focus and the carried-forward Phase 2 ownership-boundary decision.

### Product And Architecture Constraints
- `.planning/PROJECT.md` - MVP safety boundaries and non-negotiables around daemon-owned rewrites.
- `PRD.md` - Reconciliation model, AdGuard output rules, state traceability requirements, and drift policy guidance.

### Technical Guidance
- `.planning/phases/01-runtime-foundation-contracts/01-CONTEXT.md` - Locked runtime/config/contracts/state-foundation decisions from the prior phase.
- `.planning/research/ARCHITECTURE.md` - Recommended reconcile placement, apply ordering, and ownership-ledger model.
- `.planning/research/STACK.md` - Locked handwritten AdGuard client approach, atomic JSON state guidance, and API request constraints.
- `.planning/research/SUMMARY.md` - Phase-ordering rationale and the recommendation to prove output/reconcile safety before Docker watch complexity.
- `.planning/research/PITFALLS.md` - Duplicate rewrite, ownership-boundary, and state-persistence hazards that constrain this phase.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/state/store.go` already provides atomic snapshot load/save with version enforcement, so Phase 2 can build the ownership ledger update flow on a stable persistence layer.
- `internal/state/model.go` already persists `Output`, `Source`, `Hostname`, `Answer`, and `LastAppliedAt`, which is close to the minimum managed-record shape this phase needs.
- `internal/contracts/output.go` already fixes the output surface to `ListVisible`, `Create`, `Update`, and `Delete`, matching the item-level AdGuard rewrite API direction.
- `internal/contracts/source.go` already defines normalized desired records with stable source identity, which the reconcile engine can diff against output-visible and owned state.

### Established Patterns
- Runtime owns cross-cutting orchestration. `internal/runtime/app.go` already centralizes config validation, secret resolution, state-store initialization, and provider construction.
- Provider contracts are intentionally narrow and reconciliation-free, so Phase 2 should preserve that boundary and add reconcile logic above providers rather than inside them.
- The state file is already treated as durable control-plane data, so apply ordering must protect its trustworthiness.

### Integration Points
- The new reconcile core should attach to the runtime layer that already holds the state store and provider instances.
- The real AdGuard output implementation will replace `internal/providers/adguardstub/output.go` and must honor the existing output contract shape.
- Phase 3 source automation will later feed desired records into this Phase 2 reconcile engine, so the diff/apply path should be source-agnostic now.
</code_context>

<specifics>
## Specific Ideas

No additional user-specific requirements were added beyond the confirmed decisions above. Use a standard Go controller-style reconcile design and keep duplicate or ambiguous AdGuard state non-destructive by default.
</specifics>

<deferred>
## Deferred Ideas

- Docker/Godoxy label parsing breadth and startup desired-state generation stay in Phase 3.
- Event-stream reconnects, retry orchestration, and broader recovery behavior stay in Phase 4.
- Richer operator diagnostics, inspect mode, and deployment hardening stay out of this phase.

### Reviewed Todos (not folded)
None - no matching todos were present for Phase 2.
</deferred>
