# Domain Pitfalls

**Domain:** Go daemon syncing daemon-owned AdGuard Home DNS rewrites from Godoxy-compatible Docker labels
**Researched:** 2026-05-12

## Critical Pitfalls

Mistakes that cause rewrites, unsafe deletions, or chronic drift.

### Pitfall 1: Treating the Docker event stream as the source of truth
**What goes wrong:** The daemon trusts live Docker events to be complete and current, then mutates AdGuard incrementally without a durable reconciliation pass.
**Why it happens:** Docker events are useful for low-latency updates, but Docker only exposes a limited recent event backlog (`docker events --since` returns only the last 256 logged events), and streams can disconnect. If the daemon assumes “no event == no change,” it misses creates, deletes, label edits, and restarts.
**Consequences:** Stale DNS entries, missing entries after reconnect, false confidence that the system is converged, and manual cleanup becoming normal operations.
**Prevention:** Make full reconciliation the correctness mechanism and events only a hint mechanism. On startup and after any watch error/disconnect, re-list desired source state, re-list visible AdGuard state, diff, apply, then persist state. Track a last-successful reconcile point for observability, not for correctness.
**Detection:**
- Drift appears after daemon restart, Docker restart, or network blips.
- Logs show watch reconnects without an immediate reconcile.
- Operators report “works for fresh starts, breaks after outages.”

### Pitfall 2: Using mutable container names as identity instead of immutable container IDs
**What goes wrong:** Managed ownership is keyed by container name, service name, or derived hostname instead of Docker container ID.
**Why it happens:** Names are operator-friendly, but Docker emits `rename` events and names are reusable. A recreated container can inherit the old name while being a different source object.
**Consequences:** Ownership drift, accidental deletion of the new container’s records when cleaning up the old one, and inability to repair state deterministically.
**Prevention:** Use Docker container ID as `SourceObject` identity everywhere in state. Treat names, aliases, and derived domains as attributes that can change. Handle `rename`, `update`, `die`, `stop`, and `destroy` as potential recompute triggers.
**Detection:**
- State entries reference names but not IDs.
- Rename/recreate scenarios produce duplicate or missing rewrites.
- Cleanup logic cannot distinguish “same name, new container” from “same container.”

### Pitfall 3: Mutating AdGuard rewrites without a strict local ownership boundary
**What goes wrong:** The daemon infers ownership from domain names alone or assumes all matching rewrites are safe to overwrite/delete.
**Why it happens:** AdGuard Home’s rewrite list API exposes rewrite entries as domain/answer pairs; it does not provide daemon metadata for ownership tagging. That makes local persisted state the only safe authority for “what we own.”
**Consequences:** Manual operator records get overwritten or deleted, coexistence with existing AdGuard installs becomes unsafe, and the product violates its core safety promise.
**Prevention:** Never mutate a rewrite unless it is already represented in local daemon state, or the daemon is explicitly creating it for the first time during a controlled apply. State must record source object ID, managed domain, last applied answer, output identity, and timestamps/checksums useful for repair. Prefer “do nothing and warn” over guessing ownership.
**Detection:**
- Reconcile code compares desired state directly to all AdGuard rewrites.
- Delete/update logic lacks a state membership check.
- Operators hesitate to use the daemon on AdGuard instances with existing manual records.

### Pitfall 4: Creating duplicate AdGuard rewrites through naive add/delete flows
**What goes wrong:** The daemon calls `POST /control/rewrite/add` for a rule that already exists, or models every change as delete-then-add.
**Why it happens:** AdGuard’s rewrite APIs are item-level, not transactional. Real-world reports show duplicate entries can be added through the API, and delete behavior can remove all duplicates for the same domain/answer pair. That makes naive retry logic dangerous.
**Consequences:** Duplicate records, transient NXDOMAIN windows during updates, ambiguous cleanup, and retries that amplify corruption instead of healing it.
**Prevention:** Before add, compare against current visible rewrite state and local state. Prefer `PUT /control/rewrite/update` when changing an owned rule’s value. Make create/update/delete idempotent at the reconciler layer, not by trusting API semantics. Treat 200 responses from add/delete as insufficient proof of convergence; verify by re-reading state when outcomes are uncertain.
**Detection:**
- Repeated retries increase rewrite count.
- Update logic is implemented as unconditional delete/add.
- Integration tests do not cover duplicate-prevention or retry-after-partial-failure cases.

### Pitfall 5: Non-atomic state persistence causing ghost ownership or orphaned rewrites
**What goes wrong:** The daemon updates AdGuard and local state in separate unsafe steps, then crashes between them.
**Why it happens:** This project has no server-side ownership metadata, so the local state file is effectively part of the control plane. If persistence is not atomic, a crash can leave “rewrite exists but state does not” or “state says owned but rewrite is gone.”
**Consequences:** Unsafe deletes, recreating records the operator manually repaired, endless drift loops, or permanent inability to tell whether a rewrite is managed.
**Prevention:** Use atomic write+fsync+rename for the state file. Persist only fully validated snapshots. On startup, detect impossible state combinations and repair by reconciling against source desired state plus visible AdGuard state. Design an explicit recovery policy for these cases before implementation, not after the first corruption bug.
**Detection:**
- State writes happen in place.
- Empty/truncated/corrupt state files are treated as “start fresh” without safeguards.
- Crash tests produce orphaned AdGuard entries or lost ownership.

### Pitfall 6: Letting Godoxy label compatibility drift from upstream
**What goes wrong:** The daemon implements a simplified understanding of `proxy.*` labels and silently diverges from what Godoxy actually supports.
**Why it happens:** Godoxy label behavior is evolving. Upstream currently relies on `proxy.aliases`, supports exclusion semantics, and recently changed nested `proxy.*` label merge behavior for YAML object values. A homegrown parser that only matches a few examples will rot.
**Consequences:** Eligible containers are skipped, excluded containers get published, aliases resolve differently than operators expect, and upgrades to Godoxy-compatible stacks break DNS sync without obvious errors.
**Prevention:** Define an explicit MVP compatibility subset and test it against upstream examples/releases. Parse labels through a compatibility-focused normalization layer with table-driven fixtures copied from real Godoxy cases. Fail loudly on unsupported-but-present constructs instead of partially interpreting them.
**Detection:**
- Parser behavior is described as “best effort.”
- No fixture suite exists for aliases, exclusions, nested labels, and wildcard-like patterns.
- Users report “Godoxy routes it, but docker-dns-sync ignores it.”

## Moderate Pitfalls

### Pitfall 1: Wrong drift policy for manual edits to daemon-managed records
**What goes wrong:** The daemon silently adopts manual changes to an owned rewrite, or thrashes by repeatedly correcting without clear policy.
**Prevention:** Decide early that MVP is drift-to-correct, not drift-to-adopt, and log that correction explicitly. If an operator changes a daemon-owned record manually, the next reconcile should restore desired state or surface a clear warning if policy later changes.

### Pitfall 2: Triggering on the wrong container lifecycle events
**What goes wrong:** Rewrites are published on `create` before runtime eligibility is real, or removed only on `destroy` after the container already stopped being usable. Label changes and renames are missed.
**Prevention:** Treat events as recompute triggers, not direct state mutations. Re-list the affected container on `start`, `stop`, `die`, `destroy`, `rename`, `update`, and relevant `health_status` changes if health ever becomes part of eligibility.

### Pitfall 3: Ignoring AdGuard global rewrite settings
**What goes wrong:** The daemon successfully adds rewrite entries, but AdGuard global rewrite processing is disabled, so resolution still fails.
**Prevention:** Check `/control/rewrite/settings` during output initialization and surface a clear readiness warning or hard failure depending on config. Distinguish “API succeeded” from “DNS behavior is effective.”

### Pitfall 4: Domain collision handling that is implicit instead of explicit
**What goes wrong:** Two containers derive the same hostname, or a daemon-owned desired hostname collides with an existing manual rewrite, and the reconciler picks a winner accidentally.
**Prevention:** Add explicit conflict rules: owned-vs-owned collisions are an error; desired-vs-manual collisions are non-destructive warnings unless operator policy says otherwise. Never overwrite collisions by default.

### Pitfall 5: Treating retries as harmless when operations are not atomic
**What goes wrong:** Backoff/retry logic replays adds or deletes without checking current visible state first.
**Prevention:** Every retry path should begin by refreshing the relevant visible rule and consulting local ownership state. Retries should converge, not duplicate work.

## Minor Pitfalls

### Pitfall 1: Over-logging secrets and full container metadata
**What goes wrong:** AdGuard credentials, Authorization headers, or full Docker label sets leak into logs.
**Prevention:** Redact credentials by default, gate verbose provider payload logging behind debug mode, and never log secret-bearing config values.

### Pitfall 2: Weak operator diagnostics for recovery behavior
**What goes wrong:** The daemon technically reconciles, but operators cannot tell whether it is healthy, behind, or repairing drift.
**Prevention:** Emit structured logs for reconcile start/end, watch disconnects, repaired records, skipped collisions, and state-file repair actions. Expose counters/status later if needed, but logs are minimum viable observability.

### Pitfall 3: Host-binary and Docker deployment diverge operationally
**What goes wrong:** One deployment mode handles socket access, file permissions, or restart ordering differently and becomes the “broken” mode.
**Prevention:** Test both deployment modes early with the same reconcile/recovery scenarios, especially state-file permissions and Docker socket proxy connectivity.

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| Phase 1: Source normalization and label compatibility | Godoxy label parser drifts from upstream aliases/exclusions/nested labels | Freeze an MVP-compatible subset, build fixtures from upstream examples/releases, fail loudly on unsupported patterns |
| Phase 1: Source identity model | Using container names instead of IDs for ownership | Make container ID the only source identity key; treat names and aliases as mutable attributes |
| Phase 2: Reconciler core | Event-driven logic bypasses full diff/reconcile | Make startup reconcile mandatory and reconnect reconcile automatic; treat events as hints only |
| Phase 2: Conflict policy | Reconciler overwrites manual AdGuard records on hostname collision | Require explicit state ownership checks and non-destructive collision handling |
| Phase 3: AdGuard output implementation | Naive add/delete logic creates duplicates or downtime | Prefer update semantics where possible, verify visible state before retries, test duplicate edge cases |
| Phase 3: State store | Crash between AdGuard apply and state persist creates ghost/orphan state | Use atomic state snapshots and startup repair flows tested with failure injection |
| Phase 4: Recovery and resilience | Watch reconnect resumes streaming without repairing missed changes | Reconcile immediately after reconnect and alert on repeated disconnects |
| Phase 4: Deployment hardening | Docker mode and host-binary mode differ in socket/state permissions | Run parity tests for both modes and document required file/socket permissions |
| Phase 5: Observability and operator UX | Drift exists but operators cannot explain or trust daemon actions | Add structured logs for every ownership, conflict, retry, and repair decision |

## Sources

- **HIGH:** Project docs: `/home/aaron/Code/docker-dns-sync/.planning/PROJECT.md`
- **HIGH:** Project docs: `/home/aaron/Code/docker-dns-sync/PRD.md`
- **HIGH:** Docker docs — `docker system events`: https://docs.docker.com/reference/cli/docker/system/events/
- **HIGH:** AdGuard Home technical docs — rewrite APIs: https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/AGHTechDoc.md
- **HIGH:** AdGuard Home API changelog — rewrite update/settings/enabled fields: https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/openapi/CHANGELOG.md
- **MEDIUM:** Godoxy README — `proxy.aliases`, auto-discovery model: https://raw.githubusercontent.com/yusing/godoxy/main/README.md
- **MEDIUM:** Godoxy release v0.28.0 — nested `proxy.*` label merge behavior changed: https://github.com/yusing/godoxy/releases/tag/v0.28.0
- **MEDIUM:** AdGuard Home issue on duplicate rewrite API behavior: https://github.com/AdguardTeam/AdGuardHome/issues/6977
- **MEDIUM:** AdGuard Home discussion on item-level rewrite API limitations: https://github.com/AdguardTeam/AdGuardHome/discussions/5690
