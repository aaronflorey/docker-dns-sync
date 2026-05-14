# Phase 4: Recovery, Observability & Deployment - Discussion Log (Assumptions Mode)

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions captured in CONTEXT.md - this log preserves the analysis.

**Date:** 2026-05-13
**Phase:** 04-recovery-observability-deployment
**Mode:** assumptions
**Areas analyzed:** Runtime Watch Boundary, Reconnect Strategy, Output Retry Placement, Logging Scope, Deployment Artifacts

## Assumptions Presented

### Runtime Watch Boundary
| Assumption | Confidence | Evidence |
|------------|-----------|----------|
| Phase 4 should add a steady-state loop after startup reconcile and keep snapshot reconciliation as the recovery mechanism. | Confident | `.planning/ROADMAP.md`, `PRD.md`, `internal/runtime/app.go` |
| Live watch support should be optional rather than replacing the required `Source.ListDesired` contract. | Confident | `internal/contracts/source.go`, `.planning/phases/01-runtime-foundation-contracts/01-CONTEXT.md`, `PRD.md` |

### Reconnect Strategy
| Assumption | Confidence | Evidence |
|------------|-----------|----------|
| Docker event stream errors should trigger reconnect plus a reconcile pass before resuming steady-state trust in events. | Confident | `.planning/REQUIREMENTS.md`, `PRD.md`, Moby `Events` docs noted below |

### Output Retry Placement
| Assumption | Confidence | Evidence |
|------------|-----------|----------|
| Retry policy should stay above provider safety semantics and below the main runtime loop, likely as a helper or wrapper around output calls. | Likely | `internal/runtime/deps.go`, `internal/runtime/reconcile.go`, `internal/providers/adguard/output.go` |

### Logging Scope
| Assumption | Confidence | Evidence |
|------------|-----------|----------|
| Operator logs must explain startup reconcile, event-loop reconnects, retries, remote writes, and persistence without exposing credentials. | Confident | `.planning/REQUIREMENTS.md`, `PRD.md`, `internal/runtime/app.go`, `internal/providers/adguard/output.go` |

### Deployment Artifacts
| Assumption | Confidence | Evidence |
|------------|-----------|----------|
| Phase 4 should create both `systemd` and Docker deployment artifacts in-repo because none exist yet and OPS-02 requires first-class documented paths. | Confident | `.planning/ROADMAP.md`, `.planning/STATE.md`, repo root file list |

## Corrections Made

No corrections - all assumptions confirmed.

## External Research

- Moby event streams must be reopened by the caller after an error; `Events` returns messages and an error channel and stops processing when an error occurs. (Source: `https://github.com/moby/moby/blob/client/v0.4.0/client/system_events.go`)
- Docker daemon access should prefer local Unix socket, SSH, or TLS-protected connections and treat daemon credentials/socket access as privileged. (Source: `https://docs.docker.com/engine/security/protect-access/`)
- `systemd.service` recommends explicit `ExecStart=` and supports `Restart=` / `RestartSec=` for failure recovery semantics appropriate to long-running daemons. (Sources: `https://www.freedesktop.org/software/systemd/man/254/systemd.service.html`, `https://man7.org/linux/man-pages/man5/systemd.service.5.html`)
- AdGuard rewrite operations remain item-oriented list/add/delete/update endpoints, so outage recovery should reuse the current list-and-apply model rather than attempt server-side transactions. (Source: `https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/AGHTechDoc.md`)
