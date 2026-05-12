# Requirements: docker-dns-sync

**Defined:** 2026-05-12
**Core Value:** Operators get correct local DNS rewrites for eligible Docker workloads automatically, quickly, and safely without breaking manual AdGuard records.

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases.

### Configuration

- [ ] **CONF-01**: Operator can start the daemon from a TOML configuration file that defines one or more source blocks and one or more output blocks.
- [ ] **CONF-02**: Operator can configure state file location, log level, retry/backoff behavior, and provider credential references without changing code.
- [ ] **CONF-03**: Operator can run the daemon against either a local Docker socket or a Docker socket proxy using configuration alone.

### Source Discovery

- [ ] **SRC-01**: Operator can have eligible Docker containers discovered from Godoxy-compatible `proxy.*` labels.
- [ ] **SRC-02**: Operator can rely on Godoxy exclusion behavior so excluded containers do not generate DNS rewrites.
- [ ] **SRC-03**: Operator can rely on alias-derived hostname generation for common Godoxy label patterns needed to create DNS rewrites.

### Reconciliation

- [ ] **RECON-01**: Operator can start the daemon and have it perform an initial full reconciliation before relying on live Docker events.
- [ ] **RECON-02**: Operator can have daemon-managed AdGuard Home rewrites created, updated, and deleted through idempotent item-level operations.
- [ ] **RECON-03**: Operator can trust the daemon to mutate only rewrites it previously created and tracks in local state.
- [ ] **RECON-04**: Operator can keep manual or pre-existing AdGuard rewrites untouched when they are not represented in daemon state.

### State And Recovery

- [ ] **STATE-01**: Operator can restart the daemon or reboot the host and have reconciliation restore the correct managed rewrite set from source and persisted state.
- [ ] **STATE-02**: Operator can trace every daemon-managed rewrite in local state back to its source container, generated domains, output identity, and last applied value.
- [ ] **STATE-03**: Operator can recover from a Docker event stream disconnect because the daemon reconnects and runs reconciliation to repair missed changes.
- [ ] **STATE-04**: Operator can recover from temporary AdGuard Home outages because the daemon retries and converges when connectivity returns.

### Operations

- [ ] **OPS-01**: Operator can observe structured logs for startup reconciliation, event handling, state persistence, output writes, retries, and error conditions.
- [ ] **OPS-02**: Operator can deploy the daemon as either a host binary or a Docker container using documented first-class deployment paths.
- [ ] **OPS-03**: Integrator can add a future source or output implementation without changing the reconciler contract.

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Operator Experience

- **OPX-01**: Operator can run the daemon in dry-run or inspect mode to preview derived rewrites and ownership decisions before writes occur.
- **OPX-02**: Operator can inspect drift diagnostics that explain why records were created, skipped, corrected, or deleted.
- **OPX-03**: Operator can monitor daemon health through metrics or status endpoints instead of logs alone.

### Extensibility

- **EXT-01**: Integrator can add non-Godoxy sources such as Traefik, Caddy, or Nginx Proxy Manager.
- **EXT-02**: Integrator can add non-AdGuard outputs such as Pi-hole or Cloudflare.
- **EXT-03**: Operator can limit automation with domain scoping or allowlists in multi-tenant environments.

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Reverse proxy management in Godoxy | The product only synchronizes DNS rewrites and should not expand into proxy orchestration in MVP. |
| Web UI | Config and logs are sufficient for MVP; a UI adds surface area and maintenance cost. |
| Global authoritative control over all AdGuard rewrites | The daemon must preserve operator-managed records and only act on records it owns. |
| Wildcard rewrite synthesis, TTL management, and advanced policy rules | These increase ambiguity and complexity before the core sync loop is proven. |
| Automatic adoption of manual AdGuard rewrites | Ownership would become ambiguous and unsafe. |
| Cross-source conflict resolution | MVP ships one source and one output; broader conflict policy can wait for multi-provider support. |
| Network control API | Not needed for MVP and expands attack surface. |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| CONF-01 | Phase 1 | Pending |
| CONF-02 | Phase 1 | Pending |
| CONF-03 | Phase 1 | Pending |
| SRC-01 | Phase 3 | Pending |
| SRC-02 | Phase 3 | Pending |
| SRC-03 | Phase 3 | Pending |
| RECON-01 | Phase 3 | Pending |
| RECON-02 | Phase 2 | Pending |
| RECON-03 | Phase 2 | Pending |
| RECON-04 | Phase 2 | Pending |
| STATE-01 | Phase 4 | Pending |
| STATE-02 | Phase 2 | Pending |
| STATE-03 | Phase 4 | Pending |
| STATE-04 | Phase 4 | Pending |
| OPS-01 | Phase 4 | Pending |
| OPS-02 | Phase 4 | Pending |
| OPS-03 | Phase 1 | Pending |

**Coverage:**
- v1 requirements: 17 total
- Mapped to phases: 17
- Unmapped: 0 ✓

---
*Requirements defined: 2026-05-12*
*Last updated: 2026-05-12 after roadmap creation*
