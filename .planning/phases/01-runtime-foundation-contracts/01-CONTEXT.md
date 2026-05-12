# Phase 1: Runtime Foundation & Contracts - Context

**Gathered:** 2026-05-12 (assumptions mode)
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 1 establishes config-driven startup, provider wiring, and stable source/output extension points for the daemon. It locks the runtime, configuration, and contract foundations needed by later phases without pulling full reconciliation behavior, Docker discovery, or deployment hardening into scope.

</domain>

<decisions>
## Implementation Decisions

### Configuration Shape
- **D-01:** Use a single root TOML configuration model with explicit nested sections for `sources`, `outputs`, `state`, `logging`, and `retry`.
- **D-02:** Phase 1 startup must require one or more configured source blocks and one or more configured output blocks rather than hard-coding provider wiring.
- **D-03:** Docker endpoint selection, state-file location, log level, retry/backoff settings, and credential references must all be configuration-driven rather than code constants.

### Runtime Wiring Boundary
- **D-04:** Introduce a dedicated runtime/wiring layer responsible for config loading, semantic validation, lifecycle setup, and provider instantiation.
- **D-05:** Source and output implementations must be constructed by the runtime layer rather than self-bootstrap from global state or package init behavior.

### Extension Contract Scope
- **D-06:** Stable extension points are narrow in-process Go interfaces for sources and outputs.
- **D-07:** Normalization and reconciliation behavior stay outside source/output interfaces so later providers can plug into the same core contracts.
- **D-08:** Phase 1 should optimize for internal package contracts, not RPC or subprocess plugin systems.

### Identity and State Foundation
- **D-09:** Lock the local state foundation to an atomic JSON file so later phases build on one persisted ownership format instead of introducing a second persistence model.
- **D-10:** Define normalized source identity around immutable source-object IDs rather than mutable names; for Docker-backed sources this means container ID is the durable source key.
- **D-11:** Phase 1 should define the state/config contract for persisted ownership data now, even though full reconciliation logic lands in later phases.

### the agent's Discretion
- Package and file layout inside the Go module, as long as the runtime/config/contracts boundaries above stay intact.
- Whether the minimal CLI surface is a single `run` path or includes a small validation command, as long as startup remains config-driven.
- Exact validation error wording and logging field names, provided secrets stay out of logs.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase Scope
- `.planning/ROADMAP.md` - Phase 1 goal, requirements mapping, and success criteria.
- `.planning/REQUIREMENTS.md` - `CONF-01`, `CONF-02`, `CONF-03`, and `OPS-03` acceptance constraints for this phase.
- `.planning/STATE.md` - Current project focus and the existing note that MVP startup and provider wiring remain config-driven from TOML.

### Product And Architecture Constraints
- `.planning/PROJECT.md` - MVP boundaries, non-negotiables, and product-level architectural constraints.
- `PRD.md` - Architecture overview, source/output contract expectations, configuration requirements, and security constraints.

### Technical Guidance
- `.planning/research/STACK.md` - Locked stack guidance for Go, TOML parsing, logging, plugin shape, and atomic JSON persistence.
- `.planning/research/ARCHITECTURE.md` - Runtime/wiring responsibilities, narrow plugin boundaries, and normalized domain model guidance.
- `.planning/research/SUMMARY.md` - Phase-ordering rationale and the recommendation to establish config/domain/state foundations early.
- `.planning/research/PITFALLS.md` - Identity and persistence hazards that constrain the Phase 1 design.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Planning artifacts are the only reusable assets today; there is no Go implementation yet.
- `.planning/research/STACK.md` provides the concrete library and tooling shortlist Phase 1 can adopt directly.
- `.planning/research/ARCHITECTURE.md` already defines the intended runtime, plugin, and state-store seams that initial package design should mirror.

### Established Patterns
- The project is intentionally controller-shaped: configuration and runtime wiring are explicit, and later reconciliation stays centralized rather than embedded in plugins.
- Configuration is expected to be explicit and boring: one root config struct, semantic validation after decode, and no implicit multi-source config framework.
- Future extensibility is in-process and interface-based, not process-based.

### Integration Points
- Phase 1 code will anchor the daemon entrypoint, config loader, and runtime wiring used by all later phases.
- Source contracts created here will later connect to Docker/Godoxy discovery work.
- Output contracts created here will later connect to the AdGuard Home client and reconcile engine.
- State/config decisions made here will constrain later ownership-safe reconciliation and recovery work.

</code_context>

<specifics>
## Specific Ideas

No specific requirements beyond the locked decisions above. Open to standard Go package organization and a minimal daemon CLI surface.

</specifics>

<deferred>
## Deferred Ideas

- Full reconciliation behavior and safe AdGuard mutation policy - Phase 2.
- Docker/Godoxy label parsing breadth and compatibility fixtures - Phase 3.
- Event watching, reconnect logic, and runtime recovery behavior - Phase 4.
- Host-binary and container deployment packaging details - Phase 4.

### Reviewed Todos (not folded)
None - no matching todos were present for Phase 1.

</deferred>

---

*Phase: 01-runtime-foundation-contracts*
*Context gathered: 2026-05-12*
