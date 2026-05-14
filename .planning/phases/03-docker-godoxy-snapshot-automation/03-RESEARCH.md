# Phase 03: docker-godoxy-snapshot-automation - Research

**Researched:** 2026-05-13
**Domain:** Docker snapshot discovery and Godoxy-compatible DNS label derivation
**Confidence:** HIGH

## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Implement Docker/Godoxy desired-state derivation through `contracts.Source.ListDesired(context.Context)` and feed the existing startup reconcile flow.
- **D-02:** Keep Phase 3 snapshot-only; Docker watch/reconnect behavior remains Phase 4 work.
- **D-03:** Use Docker container ID as the stable `Source.ID` for all records emitted from one container.
- **D-04:** Emit one `contracts.DesiredRecord` per derived hostname while preserving shared source lineage per container.
- **D-05:** Freeze MVP compatibility to `proxy.aliases`, `proxy.exclude`, fallback-to-container-name behavior, and the DNS-relevant `proxy.<alias>.port`, `proxy.#N.port`, and `proxy.*.port` forms.
- **D-06:** Keep middleware, homepage, YAML blob labels, and broader route configuration out of Phase 3.
- **D-07:** Mirror a documented subset of Godoxy behavior rather than importing Godoxy's full parser stack.
- **D-08:** Keep Docker source config endpoint-only unless implementation proves a concrete gap.
- **D-09:** Resolve rewrite answers using supported host override first, otherwise use the derived non-local endpoint host target.

### the agent's Discretion
- Exact helper layout inside `internal/providers/docker` as long as the provider remains snapshot-focused and contract output stays deterministic.
- Whether compatibility helpers live in `source.go` or adjacent files such as `labels.go` / `container.go`.
- Exact fixture organization for Docker summary inputs and expected `DesiredRecord` outputs.

### Deferred Ideas (OUT OF SCOPE)
- Docker event watching, change hints, reconnect logic, and missed-event recovery.
- Retry orchestration around Docker or AdGuard outages.
- Non-DNS Godoxy features such as middleware and homepage metadata.
- Additional Docker-source config knobs beyond endpoint selection.

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SRC-01 | Discover eligible Docker containers from Godoxy-compatible `proxy.*` labels | Docker container listing, running-state filtering, subset label parsing |
| SRC-02 | Respect Godoxy exclusion behavior | `proxy.exclude` handling and conservative skip rules |
| SRC-03 | Generate alias-derived hostnames for common Godoxy patterns | Alias fallback, alias list expansion, `#N` and `*` port-form compatibility |
| RECON-01 | Perform initial full reconciliation before live events | Existing runtime startup reconcile path plus real Docker source output |

## Summary

Phase 03 should be split into two implementation slices: first lock the Godoxy subset in deterministic compatibility tests and parsing helpers, then implement Docker snapshot listing that turns current container metadata into `contracts.DesiredRecord` values. That matches the current repo boundary where `internal/runtime/app.go` already owns startup reconciliation and `internal/providers/docker/source.go` is the only missing desired-state source implementation. [VERIFIED: codebase grep]

Upstream Godoxy behavior is wider than this daemon needs. The safe MVP path is to mirror only the DNS-relevant subset already frozen in context: alias discovery, exclusion, fallback naming, wildcard/reference port forms, and host-target precedence. This avoids coupling Phase 03 to Godoxy's full YAML/object label parser while still covering the common label patterns called out in the PRD and roadmap. [CITED: https://raw.githubusercontent.com/yusing/godoxy/main/internal/docker/README.md] [CITED: https://raw.githubusercontent.com/yusing/godoxy/main/internal/docker/label_test.go] [CITED: https://raw.githubusercontent.com/yusing/godoxy/main/internal/route/provider/docker_test.go]

The current runtime already satisfies the startup full-sync sequence structurally: it loads owned state, calls each source's `ListDesired`, lists output-visible records, and reconciles before blocking on steady state. Phase 03 therefore does not need a second startup path; it needs a real Docker source that produces deterministic snapshots compatible with the existing reconcile engine. [VERIFIED: codebase grep]

**Primary recommendation:** Plan Phase 03 around `subset compatibility first, snapshot integration second` so label semantics are frozen before Docker-list orchestration lands.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Docker container snapshot listing | API / Backend | External Docker daemon | Source provider owns translation from Docker metadata into normalized desired records. |
| Godoxy subset parsing | API / Backend | — | Deterministic, testable label logic belongs inside the Docker provider boundary. |
| Startup full reconciliation | API / Backend | Database / Storage | Already owned by runtime and state/reconcile layers from Phase 2. |
| Ownership-safe mutation | API / Backend | Database / Storage | Already implemented; Phase 03 must feed it without changing its policy. |

## Project Constraints (from AGENTS.md)

- Use Go daemon architecture with the Docker Go SDK, TOML config, AdGuard HTTP API, and persisted local state. [VERIFIED: codebase grep]
- Preserve the safety invariant that only daemon-owned AdGuard rewrites may be mutated. [VERIFIED: codebase grep]
- Keep provider contracts narrow and avoid moving orchestration or reconcile policy into providers. [VERIFIED: codebase grep]
- Do not expose secrets or dump excessive container metadata in logs. [VERIFIED: codebase grep]
- Do not widen config unless there is a proven need. [VERIFIED: codebase grep]

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go toolchain | 1.26.3 | Runtime/compiler for daemon | Already pinned in project research and current module. [VERIFIED: codebase grep] |
| `github.com/moby/moby/client` | v0.4.1 | Docker API client with version negotiation | Already used in `internal/providers/docker/source.go`; no new dependency needed. [VERIFIED: codebase grep] |
| Docker Engine container list API | v1.54-compatible | Snapshot input surface | Phase 03 only needs container summaries, names, labels, and network/IP metadata. [CITED: https://docs.docker.com/reference/api/engine/version/v1.54/] |
| Existing `internal/contracts`, `internal/runtime`, `internal/state` | current repo | Desired-record contract and startup reconcile pipeline | Phase 03 should reuse these unchanged. [VERIFIED: codebase grep] |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| Docker API types (`container.Summary`, network settings) | current SDK | Test fixtures for source behavior | Needed for unit tests that fake container snapshots. [ASSUMED] |
| Go stdlib (`strings`, `sort`, `net`, `context`) | go1.26.3 | Label parsing, deterministic output, host/endpoint handling | Enough for the MVP subset with no extra packages. [VERIFIED: codebase grep] |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Mirrored subset parser | Import upstream Godoxy parsing packages | Pulls in broader semantics and coupling far beyond the MVP DNS subset. |
| Snapshot-first source implementation | Watch/event-first source implementation | Breaks the roadmap ordering and weakens startup correctness. |
| Existing endpoint-only config | New alias/domain/source knobs | Adds surface area before proving a concrete need. |

**Installation:**
```bash
# No new external dependencies required for Phase 03
```

## Package Legitimacy Audit

No new external packages are required for this phase. The Docker client dependency is already present and accepted in prior planning. [VERIFIED: codebase grep]

## Architecture Patterns

### System Architecture Diagram

```text
Docker API container list
          |
          v
  Docker Provider ListDesired
          |
          +--> Eligibility filter (running, not excluded, supported labels)
          |
          +--> Godoxy subset expansion
          |      - aliases / fallback name
          |      - #N and * port forms
          |      - host override precedence
          |
          v
  []contracts.DesiredRecord
          |
          v
  existing runtime startup reconcile
```

### Recommended Project Structure
```text
internal/providers/docker/
├── source.go          # provider construction + Docker snapshot listing
├── labels.go          # subset label expansion and target derivation helpers
├── source_test.go     # Docker snapshot -> DesiredRecord coverage
└── labels_test.go     # subset compatibility coverage
```

### Pattern 1: Compatibility Fixtures Before Provider Wiring
**What:** Lock the supported label subset in table-driven tests before adding container-list orchestration.
**When to use:** First implementation slice of this phase.
**Example:**
```go
func TestDeriveDesiredRecordsFromLabels(t *testing.T) {
    t.Parallel()
    // alias fallback, exclusion, wildcard/reference ports, host precedence
}
```

### Pattern 2: Provider-Owned Metadata Translation, Runtime-Owned Reconcile
**What:** Docker provider converts container metadata into normalized desired records; runtime remains unchanged.
**When to use:** `Provider.ListDesired` implementation.
**Example:**
```go
records, err := provider.ListDesired(ctx)
// runtime startup reconcile already consumes []contracts.DesiredRecord
```

### Anti-Patterns to Avoid
- **Importing full Godoxy label/YAML parser:** exceeds the frozen subset and expands scope.
- **Generating desired records from container name only when labels explicitly exclude the container:** violates `SRC-02`.
- **Non-deterministic record ordering:** creates flaky reconcile tests and noisy snapshots.
- **Adding source-side reconcile logic or state writes:** violates the existing runtime/provider boundary.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Docker transport | Raw socket HTTP | Existing Moby client | Already handles host selection and API negotiation. |
| Startup reconcile pipeline | New startup orchestration path | `internal/runtime/app.go` | Existing runtime already performs full startup reconcile correctly. |
| Full Godoxy parser | Generic object/YAML label parser | Narrow subset helper functions | Smaller, safer, and directly aligned to required DNS behaviors. |

**Key insight:** Phase 03 correctness comes from deterministic label-to-record translation, not from broader Docker automation features.

## Common Pitfalls

### Pitfall 1: Over-scoping Godoxy compatibility
**What goes wrong:** Implementation starts supporting middleware/YAML/object labels and drifts beyond the frozen subset.
**Why it happens:** Reusing upstream parser concepts wholesale.
**How to avoid:** Keep tests and helpers limited to alias, exclude, fallback naming, supported port forms, and host-target precedence.
**Warning signs:** New helpers start parsing arbitrary nested `proxy.*` fields.

### Pitfall 2: Wrong answer-target precedence
**What goes wrong:** Rewrites point at container IPs instead of the host/edge target, or ignore an explicit supported host override.
**Why it happens:** Treating port labels as the only useful route data.
**How to avoid:** Check explicit host override first; otherwise derive the default host target from the configured non-local Docker endpoint host and emit nothing when no host target exists.
**Warning signs:** Tests for `proxy.<alias>.host` or `proxy.*.host` fail or are missing.

### Pitfall 3: Snapshot output is not deterministic
**What goes wrong:** Record order varies by map iteration or container-list order, creating unstable diffs and tests.
**Why it happens:** Appending directly from unsorted labels and aliases.
**How to avoid:** Sort aliases/hostnames and final desired records before returning.
**Warning signs:** Repeated test runs produce different `DesiredRecord` ordering.

## Code Examples

### Supported subset from upstream docs
```text
proxy.aliases
proxy.exclude
proxy.<alias>.host
proxy.<alias>.port
proxy.#N.port
proxy.*.port
```
Sources: [CITED: https://raw.githubusercontent.com/yusing/godoxy/main/internal/docker/README.md] [CITED: https://raw.githubusercontent.com/yusing/godoxy/main/internal/route/provider/docker_test.go]

### Current missing seam in repo
```go
func (p *Provider) ListDesired(context.Context) ([]contracts.DesiredRecord, error) {
    return nil, nil
}
```
Source: [VERIFIED: codebase grep]

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Startup-only Docker source is still stubbed | Real snapshot derivation planned in Phase 03 | Current repo state | This phase fills the last missing startup-source seam before live watch work. |
| Broad Godoxy route parser exists upstream | MVP subset intentionally mirrored locally | Phase 03 context freeze | Reduces scope and future coupling risk. |

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Container eligibility for Phase 03 should require a running container or equivalent active state before records are emitted | Summary | Medium (could miss stopped-but-still-visible edge cases) |
| A2 | Deterministic sorting of final `DesiredRecord` output is worth locking in tests even if runtime does not strictly require order | Common Pitfalls | Low |

## Open Questions (RESOLVED)

1. **How broad should Godoxy label compatibility be in MVP?**
   - Resolved for Phase 03: support only the DNS-relevant subset from D-05 and defer all broader route/middleware behavior.

2. **Where should startup full reconciliation logic live?**
   - Resolved for Phase 03: keep it in `internal/runtime/app.go`; only implement the Docker source snapshot seam.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | Build/test/source implementation | ✓ | go1.26.3 | — |
| Docker CLI/Engine access | Optional integration verification | ✓ | 29.4.1 | Unit tests with fake client summaries |
| AdGuard Home instance | End-to-end startup reconcile verification | ✗ (not detected in this session) | — | Existing output tests + runtime unit tests |
| Upstream Godoxy docs/tests | Compatibility planning | ✓ | current upstream text | Cached planning references |

**Missing dependencies with no fallback:**
- None blocking for Phase 03 planning.
