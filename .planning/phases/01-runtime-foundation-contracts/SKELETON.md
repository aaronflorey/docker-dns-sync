# Walking Skeleton — docker-dns-sync

**Phase:** 1
**Generated:** 2026-05-12

## Capability Proven End-to-End

An operator can run `docker-dns-sync` with a TOML file, have startup validate config and secret references, instantiate stub source/output providers, initialize the atomic state store, and exit cleanly without any hard-coded runtime wiring.

## Architectural Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Framework | Go 1.26 daemon with stdlib CLI/runtime | Matches the product constraint for a small long-running infrastructure process and keeps bootstrap logic explicit. |
| Data layer | Atomic local JSON state file under `internal/state` | Satisfies D-09 and D-11 with the smallest durable ownership boundary for later reconciliation work. |
| Auth | Provider credentials resolved from env-backed secret references | Matches the PRD security rule that secrets come from config references or environment-backed values and never logs plaintext credentials. |
| Deployment target | Local development run via `go run ./cmd/docker-dns-sync -config <file>` under `mise`-pinned Go | Walking skeleton only needs a documented local full-stack command, not packaging or deployment hardening. |
| Directory layout | `cmd/docker-dns-sync` + `internal/{config,runtime,contracts,providers,state}` | Keeps runtime wiring, contracts, and persistence boundaries explicit per D-04 through D-08. |

## Stack Touched in Phase 1

- [x] Project scaffold (`go.mod`, `mise.toml`, command entrypoint, test packages)
- [x] Routing equivalent — one real CLI startup path using `-config`
- [x] Persistence — one real state-file read/write foundation with atomic replace semantics
- [x] Runtime interaction — provider factory construction wired from config
- [x] Local full-stack run command documented for the walking skeleton

## Out of Scope (Deferred to Later Slices)

- Reconciliation diff/apply logic and ownership-safe AdGuard mutations
- Real Docker source discovery, label parsing, and event watching
- Real AdGuard HTTP API behavior and rewrite settings checks
- Retry/backoff supervision beyond config contract shape
- Host-binary packaging, Docker image packaging, and deployment hardening

## Subsequent Slice Plan

Each later phase adds one vertical slice on top of this skeleton without altering its architectural decisions:

- Phase 2: Safe ownership-based reconciliation core over the locked state and provider contracts
- Phase 3: Real Docker/Godoxy desired-state snapshot generation and startup full sync
- Phase 4: Recovery, observability, and deployment parity for host-binary and Docker operators
