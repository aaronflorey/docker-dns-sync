# Phase 3: Docker/Godoxy Snapshot Automation - Discussion Log (Assumptions Mode)

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions captured in CONTEXT.md - this log preserves the analysis.

**Date:** 2026-05-13
**Phase:** 03-docker-godoxy-snapshot-automation
**Mode:** assumptions
**Areas analyzed:** Startup Reconciliation Boundary, Source Identity And Record Lineage, Godoxy Compatibility Boundary, Parsing Strategy, Desired Answer And Config Surface

## Assumptions Presented

### Startup Reconciliation Boundary
| Assumption | Confidence | Evidence |
|------------|-----------|----------|
| Phase 3 should stay snapshot-only and feed the existing `ListDesired` to startup reconcile path without adding Docker event watching yet. | Confident | `.planning/ROADMAP.md`, `internal/contracts/source.go`, `internal/runtime/app.go`, `internal/runtime/app_test.go` |

### Source Identity And Record Lineage
| Assumption | Confidence | Evidence |
|------------|-----------|----------|
| Docker-derived desired records should use container ID as stable `Source.ID`, with each alias/hostname emitted as a separate `DesiredRecord` under shared container lineage. | Confident | `.planning/phases/01-runtime-foundation-contracts/01-CONTEXT.md`, `internal/contracts/source.go`, `internal/state/model.go`, `internal/runtime/reconcile_plan.go` |

### Godoxy Compatibility Boundary
| Assumption | Confidence | Evidence |
|------------|-----------|----------|
| Freeze MVP support to the DNS-relevant subset: `proxy.aliases`, `proxy.exclude`, alias fallback to container name, and the alias/port forms `proxy.<alias>.port`, `proxy.#N.port`, and `proxy.*.port`. Treat broader route config as out of scope. | Confident | `.planning/STATE.md`, `.planning/ROADMAP.md`, `PRD.md`, upstream Godoxy docs/tests referenced below |

### Parsing Strategy
| Assumption | Confidence | Evidence |
|------------|-----------|----------|
| Mirror a documented, test-backed subset of Godoxy behavior instead of reusing the full Godoxy parser stack. | Confident | `PRD.md`, `internal/contracts/source.go`, `internal/config/model.go`, upstream Godoxy parser/docs referenced below |

### Desired Answer And Config Surface
| Assumption | Confidence | Evidence |
|------------|-----------|----------|
| Keep Docker source config endpoint-only by default, and resolve each rewrite answer using explicit supported host override first, otherwise the derived default host target. | Likely | `internal/config/model.go`, `internal/config/validate.go`, `internal/providers/docker/source.go`, `PRD.md`, upstream Godoxy docs/tests referenced below |

## Corrections Made

No corrections - all assumptions confirmed.

## External Research

- MVP Godoxy subset: support aliases, exclusion, fallback naming, and DNS-relevant port forms; broader route labels stay out of Phase 3. (Sources: `https://raw.githubusercontent.com/yusing/godoxy/main/internal/docker/README.md`, `https://raw.githubusercontent.com/yusing/godoxy/main/internal/docker/label_test.go`, `https://raw.githubusercontent.com/yusing/godoxy/main/internal/route/provider/docker_test.go`, `https://raw.githubusercontent.com/yusing/godoxy/main/examples/docker-compose/netbird.yml`)
- Parser strategy: upstream parsing is broader than this daemon needs, so mirroring a subset is safer than coupling to the full parser stack. (Sources: `https://raw.githubusercontent.com/yusing/godoxy/main/internal/docker/label.go`, `https://raw.githubusercontent.com/yusing/godoxy/main/internal/route/provider/docker.go`, `https://raw.githubusercontent.com/yusing/godoxy/main/internal/route/provider/docker_labels.yaml`)
- Answer-target rule: prefer explicit host override when present, otherwise use the derived non-local endpoint host target instead of any container-IP fallback. (Sources: `https://raw.githubusercontent.com/yusing/godoxy/main/internal/docker/README.md`, `https://raw.githubusercontent.com/yusing/godoxy/main/internal/route/route.go`, `https://raw.githubusercontent.com/yusing/godoxy/main/internal/docker/container.go`, `https://raw.githubusercontent.com/yusing/godoxy/main/internal/route/provider/docker_test.go`)
