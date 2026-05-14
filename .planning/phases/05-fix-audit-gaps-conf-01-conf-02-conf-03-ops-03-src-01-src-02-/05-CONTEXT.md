# Phase 5: fix audit gaps CONF-01, CONF-02, CONF-03, OPS-03, SRC-01, SRC-02, SRC-03, RECON-01, STATE-01, STATE-03, STATE-04, OPS-01, OPS-02 - Context

**Gathered:** 2026-05-14
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 5 closes the milestone audit gaps by fixing the real restart-recovery defect, correcting the Docker deployment evidence, and adding the missing phase-level verification artifacts that prove the already-implemented requirements from Phases 1, 3, and 4.

This phase must stay tightly scoped to audit closeout. It should not introduce new product capabilities, broaden provider support, or redesign runtime/provider contracts beyond the smallest change needed to satisfy the failed audit criteria.

</domain>

<decisions>
## Implementation Decisions

### Phase 5 Scope
- Close both real product gaps and missing audit evidence in the same phase so milestone completion is meaningful.
- Add explicit verification artifacts for Phases 1, 3, and 4 inside this closeout phase.
- Treat the milestone audit as the source-of-truth acceptance list for this phase.
- Allow limited doc and config example changes where needed to make deployment claims true.

### Restart Recovery Semantics
- When owned state exists, desired state still exists, and the managed visible rewrite is missing, reconciliation should recreate the rewrite.
- When owned state exists, desired state no longer exists, and the visible rewrite is already missing, reconciliation should drop the stale owned entry from the next managed snapshot.
- Keep duplicate-visible-record ambiguity handling non-destructive.
- Keep the fix localized to reconcile planning plus targeted tests instead of widening provider or runtime contracts.

### Deployment Evidence And Verification Style
- Fix the Docker deployment warning by making the documented container path no longer imply that the host-oriented example config is turnkey for all Docker deployments.
- Produce explicit `01-VERIFICATION.md`, `03-VERIFICATION.md`, and `04-VERIFICATION.md` artifacts with requirement-level evidence.
- Add targeted automated coverage for the restart recovery planner bug and rely on existing evidence-backed tests for the already-implemented Phase 1 and 3 behaviors.
- Update `.planning/STATE.md` and `.planning/ROADMAP.md` as part of phase completion.

### the agent's Discretion
- Exact verification artifact wording and evidence organization, as long as each audit gap is traceably closed.
- Exact deployment example layout, as long as normal container operators are no longer pointed at an invalid default AdGuard URL.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/runtime/reconcile_plan.go` already owns the desired-visible-owned diff logic, making it the right place for the restart recovery fix.
- Existing summary, validation, and verification artifacts in earlier phases provide the evidence sources needed to backfill missing phase verification docs.
- `README.md`, `testdata/config/example.toml`, and `testdata/config/socket-proxy.toml` already form the operator deployment surface and can be tightened without changing runtime behavior.

### Established Patterns
- Runtime owns reconcile policy and provider contracts stay narrow.
- Planning artifacts use phase-local `CONTEXT`, `PLAN`, `SUMMARY`, and `VERIFICATION` documents.
- Targeted table-driven Go tests are preferred over broad end-to-end additions for narrow behavioral fixes.

### Integration Points
- Restart recovery correctness flows from `internal/runtime/reconcile_plan.go` into the existing runtime reconcile/apply path.
- Deployment evidence needs README and sample-config alignment.
- Milestone acceptance depends on ROADMAP and STATE progress matching the new verification artifacts and code/doc fixes.

</code_context>

<specifics>
## Specific Ideas

- Backfill Phase 1, 3, and 4 verification files directly from existing code/tests plus any new targeted recovery test coverage added in this phase.
- Keep the recovery fix small enough that Phase 4 runtime orchestration and provider interfaces stay intact.
- Prefer a container-oriented config example or explicit container-specific override path instead of relying on `127.0.0.1` inside the Docker deployment section.

</specifics>

<deferred>
## Deferred Ideas

None - discussion stayed within the audit closeout scope.

</deferred>
