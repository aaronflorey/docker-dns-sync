# docker-dns-sync

## What This Is

`docker-dns-sync` is a Go daemon for self-hosting operators who run Godoxy-labeled Docker workloads behind AdGuard Home. It watches Docker, derives DNS rewrites from Godoxy-compatible labels, and synchronizes only daemon-owned rewrites into AdGuard Home so local DNS routing stays current without manual cleanup.

The system is intentionally built around reconciliation and local ownership tracking so it can recover safely after daemon restarts, Docker event disconnects, or host reboots without touching operator-managed AdGuard records.

## Core Value

Operators get correct local DNS rewrites for eligible Docker workloads automatically, quickly, and safely without breaking manual AdGuard records.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] Automatically discover eligible Docker containers from Godoxy-compatible labels and derive their desired DNS rewrites.
- [ ] Reconcile managed AdGuard Home rewrites on startup, runtime events, and recovery from disconnects or restarts.
- [ ] Persist local ownership state so only daemon-managed rewrites are mutated and full recovery is deterministic.
- [ ] Expose source and output plugin contracts so the reconciler can later support more providers without redesign.
- [ ] Support host-binary and Docker deployment with documented operator setup.

### Out of Scope

- Managing reverse proxy configuration inside Godoxy — the daemon only syncs DNS rewrites.
- Building a web UI in MVP — operator control is config- and log-driven.
- Shipping non-AdGuard outputs in MVP — plugin architecture must allow them later, but MVP implements AdGuard Home only.
- Shipping non-Godoxy sources in MVP — plugin architecture must allow them later, but MVP implements Docker/Godoxy only.
- Acting as a globally authoritative AdGuard rewrite controller — manual or pre-existing rewrites outside daemon state must remain untouched.
- Implementing advanced policy behavior such as wildcard synthesis, per-record TTL management, or cross-source conflict resolution — defer until the core sync loop is proven.

## Context

The project is a deterministic infrastructure daemon, not an AI system. The immediate problem is manual DNS rewrite management for Godoxy-labeled Docker workloads in AdGuard Home, which leads to stale entries, slow service availability, and fragile recovery after restarts.

The MVP architecture centers on a configuration loader, source plugins, a normalization layer, a reconciler, a local state store, and output plugins. The first source is Docker with direct parsing of Godoxy-compatible `proxy.*` labels, including alias-derived hostnames and exclusion behavior. The first output is AdGuard Home via its HTTP API using item-level rewrite operations.

Correctness depends on three behaviors: an initial full reconciliation pass on startup, event-driven updates from Docker for low latency, and reconciliation-driven recovery after event stream failures or daemon restarts. Local persisted state is the ownership boundary that prevents the daemon from modifying operator-managed AdGuard records.

## Constraints

- **Tech stack**: Go daemon using the Docker Go SDK, AdGuard Home HTTP API, TOML config, and a persisted local state file — required by the product architecture.
- **Performance**: Create, update, and delete managed rewrites within 5 seconds of normal Docker lifecycle events — this is a primary success criterion.
- **Safety**: Never modify AdGuard rewrites that are not represented in daemon state — coexistence with manual records is mandatory.
- **Recovery**: Restart, reboot, and event-stream failure recovery must converge back to correct managed state — correctness cannot rely on a perfect live stream.
- **Security**: AdGuard credentials must come from config references or environment-backed secrets and must never be logged — secrets exposure is unacceptable.
- **Operations**: MVP must support both host-binary and Docker deployment — both are first-class operator environments.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Build the MVP as a Go daemon | Strong fit for long-running infrastructure automation and Docker/HTTP integration | — Pending |
| Use Docker labels as the source of truth instead of reading Godoxy runtime state | Keeps desired state close to workloads and avoids coupling to a separate controller surface | — Pending |
| Use event-driven sync backed by reconciliation | Low-latency updates need events, but correctness across disconnects and restarts needs reconciliation | — Pending |
| Track ownership in a persisted local state file | AdGuard rewrites do not provide daemon metadata, so safe mutation requires local ownership tracking | — Pending |
| Keep plugin-oriented source and output contracts in MVP | Future expansion to more providers must not require replacing the reconciler contract | — Pending |
| Limit MVP implementations to Docker/Godoxy input and AdGuard Home output | Keeps scope focused while preserving the architecture for later extensions | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? -> Move to Out of Scope with reason
2. Requirements validated? -> Move to Validated with phase reference
3. New requirements emerged? -> Add to Active
4. Decisions to log? -> Add to Key Decisions
5. "What This Is" still accurate? -> Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check - still the right priority?
3. Audit Out of Scope - reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-05-12 after initialization*
