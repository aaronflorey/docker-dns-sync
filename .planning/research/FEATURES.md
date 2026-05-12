# Feature Landscape

**Domain:** Infrastructure DNS sync daemon for Docker label → AdGuard Home reconciliation
**Researched:** 2026-05-12

## Table Stakes

Features operators expect. Missing = product feels incomplete.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Initial full reconciliation on startup | DNS sync controllers are expected to converge from current state, not depend on a perfect live event stream. | Med | Depends on Docker source snapshot, AdGuard visible-state listing, and diff/apply logic. This is core MVP behavior. |
| Event-driven updates from Docker | Operators expect new containers and removed containers to appear or disappear quickly, not on a long polling interval. | Med | Depends on Docker watch support plus targeted re-reconciliation. Godoxy itself advertises automatic updates from container changes, so matching that expectation matters. |
| Safe ownership tracking for managed rewrites | DNS automation tools are expected to coexist with manually managed records. Unsafe global mutation is a deal-breaker. | High | This project cannot rely on provider-side metadata, so local persisted ownership state is table stakes, not a nice-to-have. |
| Idempotent create/update/delete sync | Operators expect repeat runs to be safe and no-op when nothing changed. | Med | Depends on normalized desired-record model and deterministic diffing. |
| Godoxy label compatibility for common cases | If the daemon claims Godoxy-compatible behavior, operators will expect `proxy.aliases`, container-name fallback, and exclusion semantics to work. | Med | Depends on accurate label parser and fixture coverage against upstream examples. |
| AdGuard Home rewrite CRUD support | The daemon must actually manage rewrites at item level instead of replacing the full list. | Med | Depends on `/control/rewrite/list`, `/control/rewrite/add`, `/control/rewrite/delete`, and likely `/control/rewrite/update` for efficient mutation. |
| Restart and disconnect recovery | Infrastructure controllers are expected to recover from daemon restarts, host reboots, and watch disconnects without manual cleanup. | High | Depends on persisted state, startup reconcile, and watch reconnect strategy. |
| Structured logs for reconcile decisions and failures | Operators expect log-driven debugging for headless infra daemons. | Low | Must cover startup reconcile, event handling, retries, state writes, and conflict/safety skips. |
| Config-driven deployment for host binary and Docker | Self-hosting operators expect simple file-based config and both host/Docker deployment modes. | Med | Depends on TOML config, secrets-by-reference, documented volume paths, and state-file location management. |
| Retry/backoff for AdGuard unavailability | DNS controllers are expected to converge after temporary sink outages instead of failing permanently. | Med | Depends on retry policy, bounded backoff, and replay via reconciliation. |
| Minimal safety scoping for Docker access and secrets | Operators expect least-privilege handling around Docker socket access and AdGuard credentials. | Low | MVP should support socket proxy patterns and never log secrets. |

## Differentiators

Features that set product apart. Not expected, but valued.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Dry-run / inspect mode | Lets operators validate derived records and ownership decisions before enabling writes. This is one of the highest-value post-MVP additions. | Low | Depends on core diff engine and log/report formatting. ExternalDNS explicitly recommends dry-run preflight workflows. |
| Drift diagnostics / explain output | Helps operators understand why a record exists, why it was skipped, or why it was corrected. | Med | Depends on richer reconcile result model and state provenance. |
| Metrics + health endpoints | Makes the daemon easier to monitor in homelab and small-infra setups without tailing logs. | Med | Depends on internal counters for reconcile duration, soft errors, managed record count, and watch reconnects. |
| Additional outputs (Pi-hole, Cloudflare, RFC2136, etc.) | Expands the daemon from a narrow utility into a reusable DNS sync controller. | High | Depends on stable output plugin contract and provider-specific ownership strategy. |
| Additional sources (Traefik, Caddy, Nginx Proxy Manager, static files) | Makes the system useful outside the Godoxy niche. | High | Depends on stable source plugin contract and normalization layer. |
| Conflict visibility primitives | Emitting explicit conflict or ownership-warning signals reduces fear around automation. | Med | Can start as logs, later become events/metrics/status API. |
| Domain scoping and allowlists | Lets operators limit automation to selected suffixes or records for safer multi-tenant setups. | Med | Useful once multiple sources/outputs exist; not required for initial single-purpose MVP. |
| Compatibility test suite against upstream Godoxy examples | Improves trust that label parsing will not silently drift from upstream behavior. | Med | Valuable differentiator because this project is explicitly derivative of another label contract. |

## Anti-Features

Features to explicitly NOT build.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| Global authoritative control over all AdGuard rewrites | Violates the product’s safety model and creates too much blast radius for a first release. | Only mutate records represented in local managed state. |
| Web UI in MVP | Adds surface area, auth burden, and maintenance cost without solving the core sync problem. | Keep MVP config- and log-driven; add inspect/status surfaces later only if needed. |
| Reverse proxy management inside Godoxy | Expands scope from DNS sync into full proxy orchestration. | Treat Godoxy-compatible Docker labels purely as the desired-state input contract. |
| Wildcard synthesis and advanced rewrite policy logic | Wildcards, TTL controls, reverse-DNS extras, and per-record policy rules increase ambiguity fast. | Start with deterministic one-label-to-one-derived-record logic. |
| Health-gated publication logic in MVP | Waiting on health or readiness sounds nice but increases edge cases and can diverge from Godoxy’s routing behavior. | Publish based on deterministic eligibility rules and reconcile quickly on lifecycle changes. |
| Automatic adoption of manual AdGuard records | “Adopt what already exists” sounds convenient but makes ownership ambiguous and dangerous. | Recreate or correct only daemon-owned records from local state; leave foreign records untouched. |
| Cross-source conflict resolution engine in MVP | Resolving overlaps across multiple inputs is hard to make predictable and safe. | Ship one source and one output first; defer conflict policy until multiple providers are real. |
| Network API for control in MVP | Increases attack surface for a daemon that otherwise needs none. | Use local config plus logs; consider read-only status later if operational need appears. |

## Feature Dependencies

```text
Docker container snapshot + Godoxy label parser → Normalized desired records
Normalized desired records + AdGuard visible state + persisted ownership state → Reconcile diff
Reconcile diff → Idempotent create/update/delete operations
Persisted ownership state + startup reconcile → Safe restart recovery
Docker event watch → Low-latency targeted re-reconciliation
Retry/backoff + startup/runtime reconciliation → Recovery from AdGuard outages
Structured logging + reconcile result model → Dry-run / inspect mode
Stable source/output contracts + normalized domain model → Future plugin expansion
```

## MVP Recommendation

Prioritize:
1. Initial full reconciliation with persisted ownership state
2. Event-driven Docker/Godoxy discovery with common-label compatibility
3. AdGuard Home item-level rewrite sync with safe retries and structured logs

Defer: Dry-run / inspect mode: high operator value, but it cleanly layers on top of the same diff engine and should not delay proving the core reconcile loop.

## Sources

- Project requirements: `/home/aaron/Code/docker-dns-sync/.planning/PROJECT.md` — HIGH confidence
- Product requirements: `/home/aaron/Code/docker-dns-sync/PRD.md` — HIGH confidence
- GoDoxy README, “How does GoDoxy work” and label behavior (`proxy.aliases`, watch/update automatically): https://github.com/yusing/godoxy/blob/main/README.md — HIGH confidence
- AdGuard Home technical docs, rewrite semantics and rewrite APIs: https://github.com/AdguardTeam/AdGuardHome/blob/master/AGHTechDoc.md — HIGH confidence
- AdGuard Home OpenAPI changelog, rewrite update/settings APIs: https://github.com/AdguardTeam/AdGuardHome/blob/master/openapi/CHANGELOG.md — MEDIUM confidence
- external-dns operational best practices, ownership, dry-run, events, metrics: https://kubernetes-sigs.github.io/external-dns/latest/docs/advanced/operational-best-practices/ — MEDIUM confidence
- external-dns flags reference, dry-run/events/metrics/ownership controls: https://kubernetes-sigs.github.io/external-dns/latest/docs/flags/ — MEDIUM confidence
