# Architecture Patterns

**Domain:** Event-driven infrastructure sync daemon for Docker metadata → AdGuard Home DNS rewrites
**Researched:** 2026-05-12

## Recommended Architecture

Use a **level-triggered reconciliation controller** with **event hints**, **persisted ownership state**, and **narrow source/output plugin boundaries**.

This daemon should behave more like a Kubernetes-style controller than a one-shot event processor:

1. **Sources describe desired state** (`what rewrites should exist`).
2. **Outputs expose visible state** (`what rewrites currently exist in AdGuard Home`).
3. **Local state defines ownership** (`which visible rewrites this daemon is allowed to mutate`).
4. **A reconciler computes and applies the diff** until actual state converges.
5. **Events only trigger reconciliation**; they are not the source of truth.

```text
                 +-------------------+
                 |   TOML Config     |
                 +---------+---------+
                           |
                           v
                 +---------+---------+
                 |  Runtime / Wiring |
                 | logger, backoff,  |
                 | queue, scheduler  |
                 +----+---------+----+
                      |         |
        startup/full  |         | watch hints
        snapshot      |         v
                      |   +-----+----------------+
                      |   | Docker/Godoxy Source |
                      |   | list + watch + parse |
                      |   +-----+----------------+
                      |         |
                      v         v
                +-----+---------+------+
                | Normalized Desired    |
                | Record Snapshot       |
                +-----+---------+------+
                      |         |
                      |         v
                      |   +-----+----------------+
                      |   | Local State Store    |
                      |   | ownership ledger     |
                      |   +-----+----------------+
                      |         |
                      v         v
                +-----+---------+------+
                | Reconciler / Diff     |
                | desired vs owned vs   |
                | visible output state  |
                +-----+---------+------+
                      |
                      v
                +-----+----------------+
                | AdGuard Output Plugin |
                | list/add/update/del  |
                +----------------------+
```

### Component Boundaries

| Component | Responsibility | Communicates With |
|-----------|---------------|-------------------|
| Config loader | Parse TOML, env-backed secrets, validate startup config, construct plugin instances | Runtime, source plugins, output plugins, state store |
| Runtime supervisor | Own process lifecycle, context cancellation, worker queue, retry/backoff, periodic resync scheduling | All components |
| Source plugin: Docker/Godoxy | List eligible containers, watch Docker events, parse `proxy.*` labels into normalized desired records | Docker API, normalizer, queue |
| Normalizer | Convert provider-specific metadata into canonical `DesiredRecord` values and stable source keys | Source plugin, reconciler |
| Reconcile queue | Coalesce event hints into reconcile requests (`full` or `source object` scoped) | Source plugin, runtime supervisor, reconciler |
| Local state store | Persist daemon-owned records, source linkage, last-applied values, recovery checkpoints | Reconciler, output plugin |
| Output plugin: AdGuard Home | Read visible rewrites and perform item-level create/update/delete operations | AdGuard Home HTTP API, reconciler |
| Reconciler | Read desired state, owned state, and visible output state; compute safe operations; persist new state | Source plugin, output plugin, state store |
| Observability layer | Structured logs, counters, drift/recovery diagnostics | Runtime supervisor, reconciler, plugins |

## Data Flow

### Startup flow

1. Load config and secrets.
2. Open state store and load persisted managed-record ledger.
3. Create source and output plugin instances.
4. Source performs **full desired-state listing** from Docker.
5. Output performs **visible-state listing** from AdGuard Home.
6. Reconciler computes:
   - desired records that are missing remotely → create
   - owned records with changed answers → update
   - owned records no longer desired → delete
   - owned records missing remotely but still desired → recreate
7. Persist updated ownership state only after successful output operations.
8. Start steady-state watch loop.

### Runtime flow

1. Docker event stream emits a hint (`start`, `stop`, `die`, `destroy`, `rename`, `label/network-related update` if surfaced).
2. Source maps that hint to either:
   - a specific source object reconcile, or
   - a fallback full-source resync when precision is uncertain.
3. Queue deduplicates multiple hints for the same object.
4. Reconciler re-lists the affected object or source snapshot.
5. Reconciler compares fresh desired state against persisted ownership and current AdGuard state.
6. Output applies safe item-level mutations.
7. State store is updated transactionally.

### Recovery flow

1. Docker watch disconnects, daemon restarts, or AdGuard was unavailable.
2. Runtime marks watch stream unhealthy.
3. On reconnect, perform **reconciliation before trusting live events again**.
4. Re-list source desired state and output visible state.
5. Repair drift from missed events using the state ledger as the ownership boundary.
6. Resume watch mode only after reconcile completes.

## Build Order and Dependency Implications

Build in this order:

1. **Normalized domain model + state store**
   - Needed first because every later component depends on stable identities and ownership semantics.
2. **AdGuard output plugin**
   - Needed to prove item-level rewrite CRUD and remote-state listing against the real API.
3. **Pure reconciler/diff engine**
   - Should be testable with fake source/output adapters before Docker watch complexity exists.
4. **Docker/Godoxy snapshot source**
   - Startup reconciliation is the minimum viable product for correctness.
5. **Docker watch integration + queue**
   - Low-latency updates come after the snapshot path is already correct.
6. **Recovery/backoff/resync supervisor**
   - Add reconnect and outage handling once the core reconcile path is proven.
7. **Operational polish**
   - logs, inspect mode, metrics, packaging for host-binary and container deployment.

Dependency implications:

- **Do not build watch-first.** A correct full reconcile path is the prerequisite for safe event handling.
- **Do not build output writes before visible-state reads.** Recovery requires read-before-write diffing.
- **Do not couple local state to in-memory caches.** Crash recovery depends on disk state surviving process death.
- **Do not make Docker events authoritative.** Docker streams can disconnect; the snapshot path must always be able to rebuild truth.

## Patterns to Follow

### Pattern 1: Level-triggered reconciliation
**What:** Reconcile from current desired state and current remote state, not from the specific event payload.
**When:** Always; event payloads are only scheduling hints.
**Why:** This is the standard controller pattern for surviving duplicated, missed, or delayed events. Kubernetes explicitly defines controllers as control loops that watch state and move current state toward desired state; controller-runtime further documents reconciliation as **level-based**, not event-delta-based.
**Example:**
```typescript
// Pseudocode only.
func Reconcile(scope Scope) error {
  desired := source.ListDesired(scope)
  owned := state.LoadOwned(scope)
  visible := output.ListVisible(scope)

  plan := Diff(desired, owned, visible)
  if err := output.Apply(plan); err != nil {
    return err
  }

  return state.Save(plan.NextOwnedState)
}
```

### Pattern 2: Ownership ledger as mutation authority
**What:** Persist a local ledger of every rewrite the daemon owns, including source identity and last applied target.
**When:** For every successful create/update/delete.
**Why:** AdGuard Home rewrites do not provide daemon-specific ownership metadata, so local state is the only safe boundary between daemon-managed and operator-managed records.
**Example:**
```typescript
type ManagedRecord = {
  outputKey: "adguard:domain|answer"
  sourceKey: "docker:<container-id>"
  hostname: string
  answer: string
  generationHash: string
  updatedAt: string
}
```

### Pattern 3: Snapshot + watch hybrid
**What:** Combine full-source listing with live watch updates.
**When:** Startup, reconnect, periodic resync, and normal runtime.
**Why:** Docker watch streams give latency; snapshot reconcile gives correctness.
**Example:**
```typescript
startup -> full source list -> full reconcile -> start watch
watch error -> reconnect -> full reconcile -> resume watch
timer tick -> full reconcile (safety net)
```

### Pattern 4: Narrow plugin contracts
**What:** Keep source plugins responsible for normalization and output plugins responsible for remote CRUD/listing; keep diff logic centralized.
**When:** From MVP onward.
**Why:** Future expansion to more providers should not fork reconcile behavior.
**Example:**
```typescript
interface Source {
  ListDesiredState(ctx): DesiredRecord[]
  Watch(ctx): EventHintStream
}

interface Output {
  ListVisibleState(ctx): VisibleRecord[]
  Create(ctx, record)
  Update(ctx, from, to)
  Delete(ctx, record)
}
```

### Pattern 5: Queue deduplication and scoped reconciliation
**What:** Put source-object keys or a global key into a work queue and let repeated events collapse.
**When:** Runtime event handling.
**Why:** Event storms from Docker restarts should not cause redundant writes.
**Example:**
```typescript
enqueue("container:abc123")
enqueue("container:abc123") // deduped
enqueue("full")             // used when certainty is low
```

## Anti-Patterns to Avoid

### Anti-Pattern 1: Edge-triggered mutation logic
**What:** “Container stopped, therefore delete rewrite immediately from event payload alone.”
**Why bad:** Missed or duplicated events create drift; reconnects become unsafe.
**Instead:** Re-read current source state and reconcile from snapshots.

### Anti-Pattern 2: Treating AdGuard Home as fully daemon-owned
**What:** Replace the whole rewrite list or delete everything not currently desired.
**Why bad:** Violates the product requirement to preserve manual records.
**Instead:** Mutate only records represented in local ownership state.

### Anti-Pattern 3: In-memory-only ownership tracking
**What:** Keep managed records only in RAM.
**Why bad:** Restart recovery becomes impossible and manual records become unsafe to distinguish.
**Instead:** Persist local state to disk with atomic writes.

### Anti-Pattern 4: Plugin-specific diff logic
**What:** Let each source or output decide reconciliation semantics.
**Why bad:** Future source/output expansion becomes a rewrite.
**Instead:** Keep one canonical diff engine over normalized records.

### Anti-Pattern 5: Writing state before remote success
**What:** Update local ownership first, then attempt AdGuard mutation.
**Why bad:** Crashes or HTTP failures leave false ownership claims.
**Instead:** Apply remote mutation first, then persist state, or persist via a clearly recoverable two-phase checkpoint.

## Scalability Considerations

| Concern | Small homelab (~10 containers) | Medium deployment (~100 containers / hundreds of rewrites) | Large deployment (~1K+ containers / thousands of rewrites) |
|---------|--------------------------------|------------------------------------------------------------|-------------------------------------------------------------|
| Reconcile scope | Full reconcile is cheap | Prefer object-scoped reconcile for steady state | Keep full reconcile for recovery, scoped reconcile for runtime |
| Docker event load | Direct handling is fine | Deduplicating work queue becomes important | Backpressure and bounded workers required |
| AdGuard API load | Per-item calls acceptable | Batch-like planning, serialized writes per domain helpful | Rate limiting and retry budgeting required |
| State file | Single JSON/TOML file acceptable | Atomic file writes and corruption checks needed | Consider embedded KV if state churn grows significantly |
| Drift repair | Manual inspection sufficient | Add explicit drift logs and counters | Add inspect/dry-run and repair diagnostics |

## Recommended Build-Time Interfaces

Use these domain objects early and keep them stable:

- `SourceObjectRef` — stable upstream identity, e.g. Docker container ID.
- `DesiredRecord` — hostname, answer target, source ref, generation metadata.
- `VisibleRecord` — output-visible domain/answer pair plus output-native identity if any.
- `ManagedRecord` — persisted ownership row linking source object to output record.
- `ReconcilePlan` — create/update/delete/no-op sets plus next state snapshot.

This keeps source parsing, diffing, and output mutation independently testable.

## Sources

- HIGH: Project docs: `/home/aaron/Code/docker-dns-sync/.planning/PROJECT.md`
- HIGH: Project docs: `/home/aaron/Code/docker-dns-sync/PRD.md`
- HIGH: Kubernetes controller pattern: https://kubernetes.io/docs/concepts/architecture/controller/
- HIGH: Kubernetes controller design note: https://git.k8s.io/community/contributors/devel/sig-api-machinery/controllers.md
- HIGH: controller-runtime reconcile semantics (`level-based`, deduped queue, watches enqueue reconcile requests): https://github.com/kubernetes-sigs/controller-runtime/blob/master/pkg/reconcile/reconcile.go and https://github.com/kubernetes-sigs/controller-runtime/blob/master/pkg/doc.go
- HIGH: Docker Engine API docs (API version negotiation and SDK guidance): https://docs.docker.com/reference/api/engine/
- MEDIUM: Docker events behavior and filtering surfaced in Docker docs / release notes: https://docs.docker.com/reference/api/engine/version-history/
- HIGH: AdGuard Home technical doc (`/control/rewrite/list`, `/control/rewrite/add`, `/control/rewrite/delete`): https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/AGHTechDoc.md
- HIGH: AdGuard Home API changelog (`PUT /control/rewrite/update`, rewrite settings, JSON body requirements): https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/openapi/CHANGELOG.md
- MEDIUM: GoDoxy/Godoxy README for Docker label discovery, alias behavior, and watch-style runtime model: https://raw.githubusercontent.com/yusing/godoxy/main/README.md
