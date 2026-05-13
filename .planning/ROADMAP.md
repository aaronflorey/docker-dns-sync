# Roadmap: docker-dns-sync

## Overview

This MVP roadmap delivers a safe Docker-to-AdGuard reconciliation daemon in four phases: first make configuration and extension boundaries stable, then prove ownership-safe reconciliation, then add Docker/Godoxy discovery with startup sync, and finally harden recovery, observability, and deployment so operators can trust it in normal failures.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: Runtime Foundation & Contracts** - Config-driven startup, provider wiring, and stable plugin boundaries.
- [ ] **Phase 2: Ownership-Safe Reconciliation Core** - Persist ownership and safely mutate only daemon-managed AdGuard rewrites.
- [ ] **Phase 3: Docker/Godoxy Snapshot Automation** - Derive desired rewrites from labeled containers and perform startup full sync.
- [ ] **Phase 4: Recovery, Observability & Deployment** - Keep the daemon correct through restarts, disconnects, outages, and real operator deployment paths.

## Phase Details

### Phase 1: Runtime Foundation & Contracts
**Goal**: Operators can configure and start the daemon with stable source/output extension points.
**Mode:** mvp
**Depends on**: Nothing (first phase)
**Requirements**: CONF-01, CONF-02, CONF-03, OPS-03
**Success Criteria** (what must be TRUE):
  1. Operator can start the daemon from a TOML file that defines one or more sources and one or more outputs.
  2. Operator can change state file location, log level, retry/backoff behavior, credential references, and Docker endpoint selection through configuration alone.
  3. Operator can point the same daemon at either a local Docker socket or a Docker socket proxy without code changes.
  4. Integrator can add a new source or output implementation behind the existing contracts without changing the reconciler contract.
**Plans**: 4
Plans:
- [x] 01-01-PLAN.md — Bootstrap the Go module, CLI run path, and minimal TOML startup smoke test.
- [x] 01-02-PLAN.md — Add semantic config validation, secret resolution, and Docker endpoint mode coverage.
- [x] 01-03-PLAN.md — Define stable source/output contracts and runtime-owned factories with real Docker endpoint bootstrap.
- [x] 01-04-PLAN.md — Lock the atomic JSON state foundation and wire runtime logging, retry, and startup state initialization.

### Phase 2: Ownership-Safe Reconciliation Core
**Goal**: Operators can trust the daemon to reconcile AdGuard rewrites without touching records it does not own.
**Mode:** mvp
**Depends on**: Phase 1
**Requirements**: RECON-02, RECON-03, RECON-04, STATE-02
**Success Criteria** (what must be TRUE):
  1. Reconciliation can create, update, and delete daemon-managed AdGuard rewrites through idempotent item-level operations.
  2. Manual or pre-existing AdGuard rewrites that are not represented in daemon state remain unchanged after reconciliation.
  3. The daemon only mutates rewrites it previously created and tracks in local state.
  4. Operator can trace every daemon-managed rewrite in local state back to its source container, generated domains, output identity, and last applied value.
**Plans**: 2

Plans:
- [ ] 02-01-PLAN.md — Build the runtime reconcile planner/apply slice with ownership-gated tests and state ordering coverage.
- [ ] 02-02-PLAN.md — Replace the AdGuard stub with a real output provider and runtime factory wiring.

### Phase 3: Docker/Godoxy Snapshot Automation
**Goal**: Operators can get the correct desired rewrite set from Docker/Godoxy labels during initial synchronization.
**Mode:** mvp
**Depends on**: Phase 2
**Requirements**: SRC-01, SRC-02, SRC-03, RECON-01
**Success Criteria** (what must be TRUE):
  1. Eligible Docker containers with Godoxy-compatible `proxy.*` labels are discovered and converted into desired DNS rewrites.
  2. Containers covered by Godoxy exclusion behavior do not generate DNS rewrites.
  3. Common Godoxy alias label patterns generate the expected derived hostnames for rewrite creation.
  4. When the daemon starts, it performs an initial full reconciliation from current source state before relying on live Docker events.
**Plans**: TBD

### Phase 4: Recovery, Observability & Deployment
**Goal**: Operators can keep the daemon converged across failures and run it in their preferred deployment mode.
**Mode:** mvp
**Depends on**: Phase 3
**Requirements**: STATE-01, STATE-03, STATE-04, OPS-01, OPS-02
**Success Criteria** (what must be TRUE):
  1. After a daemon restart or host reboot, reconciliation restores the correct managed rewrite set from source and persisted state.
  2. After a Docker event stream disconnect, the daemon reconnects and runs reconciliation to repair missed changes.
  3. After a temporary AdGuard Home outage, the daemon retries and converges when connectivity returns.
  4. Operator can observe structured logs for startup reconciliation, event handling, state persistence, output writes, retries, and error conditions.
  5. Operator can deploy the daemon as either a host binary or a Docker container using documented first-class setup paths.
**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Runtime Foundation & Contracts | 4/4 | Complete | 2026-05-12 |
| 2. Ownership-Safe Reconciliation Core | 0/TBD | Not started | - |
| 3. Docker/Godoxy Snapshot Automation | 0/TBD | Not started | - |
| 4. Recovery, Observability & Deployment | 0/TBD | Not started | - |
