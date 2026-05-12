# Project Research Summary

**Project:** docker-dns-sync
**Domain:** Infrastructure reconciliation daemon for Docker-to-DNS sync
**Researched:** 2026-05-12
**Confidence:** HIGH

## Executive Summary

`docker-dns-sync` should be built as a small, boring Go controller: read Docker state, derive Godoxy-compatible DNS intent, compare it against AdGuard Home’s visible rewrites plus a local ownership ledger, and reconcile toward convergence. The research is consistent across stack, features, architecture, and pitfalls: correctness comes from full reconciliation and persisted ownership state, while Docker events are only there to reduce latency.

The recommended MVP is narrow on purpose. Use Go 1.26, the official Moby client, a handwritten AdGuard HTTP client, TOML config, structured logs, and a single atomic JSON state file. Ship one source (Docker/Godoxy) and one output (AdGuard Home), with plugin boundaries kept in-process and small. Build snapshot reconcile first, then add event-triggered updates, then resilience and operator polish.

The main risk is unsafe mutation: deleting or overwriting AdGuard records the daemon does not truly own, or drifting because Docker events were treated as authoritative. Mitigation is equally clear: key state by immutable container ID, persist ownership atomically, reconcile from fresh source/output snapshots on startup and reconnect, and make collisions or uncertain ownership non-destructive by default.

## Key Findings

### Recommended Stack

The stack recommendation is deliberately conservative. This is a single-process infrastructure daemon, not a platform. The best fit is modern Go plus the standard library, a tiny amount of explicit integration code, and no database or heavyweight framework until scale proves the need.

**Core technologies:**
- **Go 1.26.3**: primary runtime — strong fit for a long-running daemon, static builds, good concurrency, low operational overhead.
- **Moby Docker client (`github.com/moby/moby/client`)**: Docker snapshot/watch integration — avoids reimplementing socket HTTP, version negotiation, and event streaming.
- **Handwritten AdGuard Home `net/http` client**: rewrite CRUD and settings access — API surface is small enough that a generated SDK would add more churn than value.
- **`BurntSushi/toml`**: config decoding — explicit, predictable, and better suited than multi-source config frameworks.
- **Atomic JSON state file**: ownership persistence — transparent, single-writer, and sufficient for safe restart/recovery semantics.
- **`log/slog`**: structured logs — enough observability for MVP without extra dependencies.

**Critical version requirements:**
- Pin `go 1.26` with `toolchain go1.26.3`.
- Target Docker Engine API v1.54 via current Moby client.
- Assume AdGuard Home rewrite APIs current through `v0.107.72`, including `/rewrite/update` and `/rewrite/settings` behavior.

### Expected Features

The MVP table stakes are about safe convergence, not breadth. If startup reconciliation, ownership tracking, idempotent CRUD, Docker event updates, restart recovery, and structured logs are missing, the product will feel incomplete even if the happy path works.

**Must have (table stakes):**
- Initial full reconciliation on startup.
- Event-driven Docker updates with targeted re-reconcile.
- Safe ownership tracking for daemon-managed rewrites.
- Idempotent create/update/delete sync against AdGuard Home.
- Godoxy label compatibility for common cases, especially aliases and exclusions.
- Restart/disconnect recovery with persisted state.
- Config-driven deployment for host binary and Docker.
- Retry/backoff for temporary AdGuard outages.

**Should have (competitive, but post-MVP):**
- Dry-run / inspect mode.
- Drift diagnostics / explain output.
- Metrics and health endpoints.
- Compatibility fixture suite against upstream Godoxy examples.

**Defer (v2+):**
- Additional outputs such as Pi-hole, Cloudflare, or RFC2136.
- Additional sources such as Traefik, Caddy, or Nginx Proxy Manager.
- Domain scoping, richer conflict surfaces, and any network control API.

### Architecture Approach

The architecture should follow a Kubernetes-style, level-triggered controller pattern. Source plugins describe desired state, output plugins expose visible state, a local ledger defines ownership, and a centralized reconciler computes safe create/update/delete operations. Events enqueue work; they do not encode truth.

**Major components:**
1. **Config loader + runtime supervisor** — validates TOML, wires dependencies, owns lifecycle, retries, queueing, and resync scheduling.
2. **Docker/Godoxy source + normalizer** — lists eligible containers, watches Docker, and converts `proxy.*` labels into canonical desired records keyed by container ID.
3. **Reconciler + local state store** — diffs desired state, visible AdGuard state, and owned state; applies mutations; persists ownership atomically.
4. **AdGuard output plugin** — lists rewrites, checks rewrite settings, and performs item-level add/update/delete safely.
5. **Observability layer** — structured logs now; counters/health later.

### Critical Pitfalls

1. **Treating Docker events as the source of truth** — always reconcile from fresh snapshots on startup, reconnect, and recovery.
2. **Using mutable container names as identity** — key ownership and source references by immutable container ID only.
3. **Mutating AdGuard without a strict local ownership boundary** — never touch rewrites outside daemon state except initial controlled creates.
4. **Naive add/delete flows causing duplicate rewrites** — prefer update semantics, read visible state before retries, and make operations idempotent in the reconciler.
5. **Non-atomic state persistence** — write temp file, fsync, rename, and repair from source/output truth on startup when state is suspect.

## Implications for Roadmap

Based on research, suggested phase structure:

### Phase 1: Domain Model, Config, and Ownership State
**Rationale:** Everything else depends on stable identities, normalized records, and safe persistence semantics.
**Delivers:** TOML config loading/validation, core domain types, atomic JSON state store, ownership ledger keyed by container ID.
**Addresses:** Safe ownership tracking, config-driven deployment, deterministic recovery foundation.
**Avoids:** Container-name identity drift; ghost/orphan state from unsafe persistence.

### Phase 2: AdGuard Output and Reconcile Engine
**Rationale:** Output read/write semantics and the diff engine are the real safety core; prove them before Docker watch complexity.
**Delivers:** AdGuard visible-state listing, rewrite CRUD/update path, centralized diff planner, collision handling, idempotent apply flow.
**Uses:** Go stdlib HTTP, handwritten AdGuard client, structured logs.
**Implements:** Output plugin + reconciler architecture.
**Avoids:** Global rewrite control, duplicate rewrite creation, unsafe manual-record overwrites.

### Phase 3: Docker/Godoxy Snapshot Source
**Rationale:** A correct full-source snapshot path is the minimum viable correctness path and should exist before live events.
**Delivers:** Docker container listing, Godoxy-compatible label parsing for MVP subset, normalized desired record generation, startup full reconcile.
**Addresses:** Initial full reconciliation, common-label compatibility, core MVP automation.
**Avoids:** Watch-first design and silent label-compatibility drift.

### Phase 4: Watch, Queue, and Recovery Resilience
**Rationale:** Low-latency updates come after correctness; resilience comes after both.
**Delivers:** Docker event watch, deduplicating queue, scoped re-reconcile, reconnect logic, retry/backoff, periodic safety resync.
**Addresses:** Event-driven updates, restart/disconnect recovery, AdGuard outage recovery, sub-5-second steady-state sync.
**Avoids:** Edge-triggered mutations, missed-event drift, retry amplification bugs.

### Phase 5: Operational Polish and Validation
**Rationale:** Once the control loop is correct, improve operator trust and release readiness.
**Delivers:** Deployment packaging for host binary and container, integration/compatibility tests, stronger diagnostics, optional dry-run/inspect if scope permits.
**Addresses:** Structured logging, host/Docker parity, confidence in upstream Godoxy compatibility.
**Avoids:** Operational divergence between deployment modes and opaque recovery behavior.

### Phase Ordering Rationale

- The research is aligned that **watch-first is the wrong order**; snapshot reconcile and ownership semantics must exist before live event handling.
- Output visible-state reads must precede robust writes because recovery depends on read-before-write diffing.
- The roadmap groups work around architecture boundaries: state and model first, then output/reconcile core, then source discovery, then watch/recovery, then operator polish.
- This order directly lowers the biggest risks: unsafe ownership, duplicate rewrites, missed Docker events, and label-compatibility drift.

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 3:** Exact MVP Godoxy compatibility subset, especially aliases, exclusions, and any nested `proxy.*` behaviors.
- **Phase 5:** Packaging and operational parity details for host-binary vs Docker deployment, including socket proxy and state-file permissions.

Phases with standard patterns (skip research-phase):
- **Phase 1:** Config parsing, domain modeling, and atomic file persistence are well-understood.
- **Phase 2:** Controller-style diff/apply logic and a small explicit HTTP client are well-documented patterns.
- **Phase 4:** Work queue deduplication, reconnect, and bounded backoff follow established controller practices.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Strongly grounded in official Go, Docker, and AdGuard docs; recommendations are intentionally narrow and low-risk. |
| Features | HIGH | Closely aligned with PROJECT.md and PRD.md, with differentiators clearly separated from MVP requirements. |
| Architecture | HIGH | Based on well-established controller patterns plus explicit project constraints around ownership and reconciliation. |
| Pitfalls | HIGH | Major risks are concrete, repeated across sources, and tied to known Docker/AdGuard behavior. |

**Overall confidence:** HIGH

### Gaps to Address

- **Godoxy compatibility boundary:** Freeze the exact MVP-supported label subset during technical design and back it with fixtures from upstream examples/releases.
- **Desired rewrite target semantics:** Confirm whether generated AdGuard answers always map to container IPs or may need operator-defined host targets in some flows.
- **Manual edits to daemon-owned records:** Keep MVP policy as drift-to-correct, but document it explicitly and test the operator experience.
- **AdGuard duplicate/update edge cases:** Validate behavior with integration tests against the real API, especially around retries and partial failures.

## Sources

### Primary (HIGH confidence)
- `/home/aaron/Code/docker-dns-sync/.planning/PROJECT.md` — product scope, constraints, and MVP boundaries.
- `/home/aaron/Code/docker-dns-sync/PRD.md` — acceptance criteria, architecture requirements, roadmap intent, and open questions.
- https://go.dev/doc/go1.26 and https://go.dev/VERSION?m=text — Go version guidance.
- https://docs.docker.com/reference/api/engine/ and https://docs.docker.com/reference/api/engine/sdk/examples/ — Docker API and Moby client guidance.
- https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/AGHTechDoc.md — AdGuard rewrite API semantics.
- https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/openapi/CHANGELOG.md — AdGuard rewrite update/settings behavior.
- https://kubernetes.io/docs/concepts/architecture/controller/ — controller reconciliation pattern.

### Secondary (MEDIUM confidence)
- https://raw.githubusercontent.com/yusing/godoxy/main/README.md — Godoxy label behavior and compatibility expectations.
- https://github.com/yusing/godoxy/releases/tag/v0.28.0 — evolving nested label behavior that may affect compatibility scope.
- https://kubernetes-sigs.github.io/external-dns/latest/docs/advanced/operational-best-practices/ — dry-run, ownership, and controller operational practices.
- https://github.com/AdguardTeam/AdGuardHome/issues/6977 and https://github.com/AdguardTeam/AdGuardHome/discussions/5690 — duplicate rewrite and API edge-case evidence.

### Tertiary (LOW confidence)
- None identified; the main open issues are scope decisions, not weak sourcing.

---
*Research completed: 2026-05-12*
*Ready for roadmap: yes*
