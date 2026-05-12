# Phase 1: Runtime Foundation & Contracts - Research

**Researched:** 2026-05-12
**Domain:** Go daemon runtime bootstrap, config contracts, provider factories, secret handling, atomic state foundation
**Confidence:** HIGH

## User Constraints (from CONTEXT.md)

### Locked Decisions

### Configuration Shape
- **D-01:** Use a single root TOML configuration model with explicit nested sections for `sources`, `outputs`, `state`, `logging`, and `retry`.
- **D-02:** Phase 1 startup must require one or more configured source blocks and one or more configured output blocks rather than hard-coding provider wiring.
- **D-03:** Docker endpoint selection, state-file location, log level, retry/backoff settings, and credential references must all be configuration-driven rather than code constants.

### Runtime Wiring Boundary
- **D-04:** Introduce a dedicated runtime/wiring layer responsible for config loading, semantic validation, lifecycle setup, and provider instantiation.
- **D-05:** Source and output implementations must be constructed by the runtime layer rather than self-bootstrap from global state or package init behavior.

### Extension Contract Scope
- **D-06:** Stable extension points are narrow in-process Go interfaces for sources and outputs.
- **D-07:** Normalization and reconciliation behavior stay outside source/output interfaces so later providers can plug into the same core contracts.
- **D-08:** Phase 1 should optimize for internal package contracts, not RPC or subprocess plugin systems.

### Identity and State Foundation
- **D-09:** Lock the local state foundation to an atomic JSON file so later phases build on one persisted ownership format instead of introducing a second persistence model.
- **D-10:** Define normalized source identity around immutable source-object IDs rather than mutable names; for Docker-backed sources this means container ID is the durable source key.
- **D-11:** Phase 1 should define the state/config contract for persisted ownership data now, even though full reconciliation logic lands in later phases.

### the agent's Discretion
- Package and file layout inside the Go module, as long as the runtime/config/contracts boundaries above stay intact.
- Whether the minimal CLI surface is a single `run` path or includes a small validation command, as long as startup remains config-driven.
- Exact validation error wording and logging field names, provided secrets stay out of logs.

### Deferred Ideas (OUT OF SCOPE)
- Full reconciliation behavior and safe AdGuard mutation policy - Phase 2.
- Docker/Godoxy label parsing breadth and compatibility fixtures - Phase 3.
- Event watching, reconnect logic, and runtime recovery behavior - Phase 4.
- Host-binary and container deployment packaging details - Phase 4.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| CONF-01 | Operator can start daemon from TOML with 1+ sources and 1+ outputs | Config schema, semantic validation, provider factory contracts, startup wiring sequence |
| CONF-02 | Operator can configure state path, log level, retry/backoff, credential refs | Explicit root config model, secret-reference pattern, runtime options injection |
| CONF-03 | Operator can target local Docker socket or socket proxy via config | Endpoint config contract (`unix://`, `tcp://`, SSH/TLS-ready fields), source factory inputs |
| OPS-03 | Integrator can add future source/output without reconciler contract changes | Narrow source/output interfaces + registry-based factories + centralized runtime construction |
</phase_requirements>

## Summary

Phase 1 should deliver a **thin walking skeleton**: a daemon entrypoint that loads TOML, validates semantics, resolves secrets safely, instantiates source/output providers through factories, initializes atomic state storage, and starts a minimal runtime loop (even if reconciliation logic is still stubbed). [CITED: /home/aaron/Code/docker-dns-sync/.planning/phases/01-runtime-foundation-contracts/01-CONTEXT.md] [CITED: /home/aaron/Code/docker-dns-sync/.planning/ROADMAP.md]

The most important planning risk is accidentally leaking Phase 2+ concerns (full diff engine, Docker watch behavior, AdGuard write semantics) into this phase. Phase 1 should instead lock contracts and runtime seams so later phases can plug in behavior safely without redesign. [CITED: /home/aaron/Code/docker-dns-sync/.planning/ROADMAP.md] [CITED: /home/aaron/Code/docker-dns-sync/.planning/research/ARCHITECTURE.md]

**Primary recommendation:** Plan Phase 1 around `bootstrap correctness` (config + factories + secrets + atomic state I/O + contract tests), not around sync behavior. [CITED: /home/aaron/Code/docker-dns-sync/.planning/REQUIREMENTS.md]

## Project Constraints (from AGENTS.md)

- Use Go daemon architecture with Docker SDK, AdGuard HTTP API, TOML config, and persisted local state file. [CITED: /home/aaron/Code/docker-dns-sync/AGENTS.md]
- Preserve safety invariant: never modify non-daemon AdGuard rewrites. [CITED: /home/aaron/Code/docker-dns-sync/AGENTS.md]
- Keep credential material out of logs; use config references/environment-backed secrets. [CITED: /home/aaron/Code/docker-dns-sync/AGENTS.md]
- Maintain host-binary and Docker deployment as first-class operations constraints (even if full packaging is later). [CITED: /home/aaron/Code/docker-dns-sync/AGENTS.md]
- Follow GSD workflow discipline; this document is for planning artifacts under `.planning/phases/...`. [CITED: /home/aaron/Code/docker-dns-sync/AGENTS.md]

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Config load + decode + semantic validation | API / Backend | — | Daemon process owns startup correctness and should fail-fast before runtime starts. |
| Secret reference resolution | API / Backend | OS/runtime env | Secret names in config resolve to env-backed values at process start. |
| Source provider factory | API / Backend | — | Provider construction is runtime wiring responsibility, not plugin self-bootstrap. |
| Output provider factory | API / Backend | — | Same boundary as sources; central runtime controls dependencies and lifecycle. |
| Atomic state persistence contract | Database / Storage | API / Backend | State file is persistence tier; runtime invokes transactional read/write operations. |
| Docker endpoint selection contract | API / Backend | External Docker daemon | Config-driven endpoint targeting local socket or proxy is daemon-side concern. |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go toolchain | 1.26.3 target | Runtime/language | Project stack guidance is pinned to Go 1.26.x for reproducible daemon builds. [CITED: /home/aaron/Code/docker-dns-sync/.planning/research/STACK.md] |
| `github.com/BurntSushi/toml` | v1.6.0 | TOML decode | Explicit struct decode with low magic; fits config-heavy infra daemon startup. [VERIFIED: npm registry] [CITED: https://github.com/BurntSushi/toml] |
| Go stdlib `log/slog` | go1.26 stdlib | Structured logging | Sufficient for MVP operational logging without extra dependencies. [CITED: /home/aaron/Code/docker-dns-sync/.planning/research/STACK.md] |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/moby/moby/client` | v0.4.1 | Docker client construction surface | Needed now for endpoint/config contract shape and source factory inputs, even before full discovery features. [VERIFIED: npm registry] [CITED: https://docs.docker.com/reference/api/engine/sdk/examples/] |
| `github.com/cenkalti/backoff/v5` | v5.0.3 | Retry policy structs | Optional in Phase 1; include only if runtime wiring must parse/validate retry policy object now. [ASSUMED] |
| `github.com/testcontainers/testcontainers-go` | v0.42.0 | Integration tests | Defer for later phase integration unless Phase 1 adds executable provider bootstrap checks. [ASSUMED] |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `BurntSushi/toml` | `spf13/viper` | Viper adds implicit precedence and dynamic config behavior that harms startup determinism. [CITED: /home/aaron/Code/docker-dns-sync/.planning/research/STACK.md] |
| Factory registries | Direct switch/case in main | Switch is simpler initially, but registries reduce edit hotspots when adding providers. [ASSUMED] |

**Installation:**
```bash
go get github.com/BurntSushi/toml@v1.6.0
go get github.com/moby/moby/client@v0.4.1
```

**Version verification:**
- `go list -m -json github.com/BurntSushi/toml@latest` → `v1.6.0` (`2025-12-18T12:15:22Z`) [VERIFIED: npm registry]
- `go list -m -json github.com/moby/moby/client@latest` → `v0.4.1` (`2026-04-20T14:44:55Z`) [VERIFIED: npm registry]
- `go list -m -json github.com/cenkalti/backoff/v5@latest` → `v5.0.3` (`2025-07-23T16:23:35Z`) [VERIFIED: npm registry]
- `go list -m -json github.com/testcontainers/testcontainers-go@latest` → `v0.42.0` (`2026-04-09T15:22:39Z`) [VERIFIED: npm registry]

## Package Legitimacy Audit

| Package | Registry | Age | Downloads | Source Repo | slopcheck | Disposition |
|---------|----------|-----|-----------|-------------|-----------|-------------|
| github.com/BurntSushi/toml | Go module proxy | Mature | n/a | github.com/BurntSushi/toml | SLOP* | Approved with manual review |
| github.com/moby/moby/client | Go module proxy | Mature | n/a | github.com/moby/moby | SLOP* | Approved with manual review |
| github.com/cenkalti/backoff/v5 | Go module proxy | Mature | n/a | github.com/cenkalti/backoff | SLOP* | Optional, manual review |
| github.com/testcontainers/testcontainers-go | Go module proxy | Mature | n/a | github.com/testcontainers/testcontainers-go | SLOP* | Deferred for this phase |

\* `slopcheck install` in this environment validates against PyPI and returned false `[SLOP]` for Go module import paths; use `go list -m ...@latest` and official docs as authoritative checks for Go packages. [VERIFIED: npm registry]

**Packages removed due to slopcheck [SLOP] verdict:** none (false positives due to ecosystem mismatch)
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```text
TOML file
  |
  v
Config Loader ----> Semantic Validator ----> Secret Resolver
  |                        |                       |
  |                        v                       v
  |------------------> Runtime Builder <------ Provider Registry
                             |
                             +--> Source Factory (1..N)
                             +--> Output Factory (1..N)
                             +--> State Store (atomic JSON)
                             |
                             v
                         Run Loop Stub
                    (health/log/lifecycle only)
```

### Recommended Project Structure
```text
cmd/docker-dns-sync/
  main.go                 # cli entrypoint
internal/runtime/
  app.go                  # wiring/bootstrap lifecycle
  factories.go            # provider factory registry + constructors
internal/config/
  model.go                # root config structs
  load.go                 # toml load + decode
  validate.go             # semantic validation rules
  secrets.go              # credential reference resolution
internal/contracts/
  source.go               # Source interface + identity types
  output.go               # Output interface + identity types
internal/state/
  model.go                # managed-state schema
  store.go                # read/write API
  atomic_file.go          # temp+fsync+rename implementation
```

### Pattern 1: Two-Phase Config Validation
**What:** Decode TOML into typed struct, then run semantic checks (required sections, cardinality, endpoint forms, mutually exclusive secret fields).
**When to use:** Every startup path and optional `validate-config` command.
**Example:**
```go
// Source: https://github.com/BurntSushi/toml
if _, err := toml.DecodeFile(path, &cfg); err != nil { return err }
if err := cfg.Validate(); err != nil { return err }
```

### Pattern 2: Provider Registry + Factory Construction
**What:** Runtime owns provider type maps (`type -> constructor`) and builds instances from validated config blocks.
**When to use:** To satisfy OPS-03 extensibility with minimal churn.
**Example:**
```go
type SourceFactory func(context.Context, SourceConfig, RuntimeDeps) (contracts.Source, error)
var sourceFactories = map[string]SourceFactory{}
```

### Anti-Patterns to Avoid
- **Global singleton bootstrap in provider packages:** breaks D-05 runtime ownership boundary.
- **Config defaults hidden in code constants:** violates CONF-02/CONF-03 configuration-driven behavior.
- **Inline secret values in logs/errors:** violates project security constraint.
- **State writes before validation completion:** risks corrupt ownership foundation.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| TOML parser | Custom parser | `BurntSushi/toml` | TOML edge cases are non-trivial; mature parser reduces startup bugs. |
| Docker transport | Raw socket HTTP client | `moby/moby/client` | Handles transport and version negotiation correctly. |
| Atomic file semantics | Naive `os.WriteFile` overwrite | temp + fsync + rename flow | Prevents torn/truncated state under crash/power loss patterns. |

**Key insight:** Phase 1 safety comes from predictable primitives, not custom infrastructure.

## Common Pitfalls

### Pitfall 1: Accepting structurally valid but semantically unusable config
**What goes wrong:** TOML parses but runtime lacks source/output blocks or required fields.
**Why it happens:** Decode-only validation.
**How to avoid:** Explicit semantic validation with requirement-aligned checks (`CONF-01/02/03`).
**Warning signs:** Startup panics downstream in factory construction.

### Pitfall 2: Blurring contract vs implementation responsibilities
**What goes wrong:** Interfaces embed reconciliation behavior too early.
**Why it happens:** Trying to “future-proof” with fat contracts.
**How to avoid:** Keep source/output interfaces narrow; reconciliation remains centralized in later phases.
**Warning signs:** Interface methods mention diff/apply semantics.

### Pitfall 3: Secret-reference ambiguity
**What goes wrong:** Config allows both inline credential and env reference with unclear precedence.
**Why it happens:** Missing validation invariants.
**How to avoid:** Require exactly one credential source per secret field.
**Warning signs:** Startup warnings instead of hard failures for auth ambiguity.

## Code Examples

### Atomic state write pattern
```go
// Source: https://pkg.go.dev/os#Rename
tmp := statePath + ".tmp"
if err := os.WriteFile(tmp, payload, 0o600); err != nil { return err }
if err := syncFile(tmp); err != nil { return err }
if err := os.Rename(tmp, statePath); err != nil { return err }
```

### Thin walking skeleton startup sequence
```go
cfg := config.Load(path)
config.Validate(cfg)
resolved := config.ResolveSecrets(cfg, os.LookupEnv)
app := runtime.Build(resolved)
return app.Run(ctx)
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Monolithic main with hard-coded providers | Runtime wiring layer + config-driven factories | Modern Go daemon/controller practice [ASSUMED] | Easier extension and testability |
| Event-first assumptions | Reconcile-first architecture | Established in project architecture docs | Better correctness under failures |

**Deprecated/outdated:**
- Heavy dynamic config frameworks for this daemon class are discouraged in project stack guidance. [CITED: /home/aaron/Code/docker-dns-sync/.planning/research/STACK.md]

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `backoff/v5` should be included in Phase 1 wiring | Standard Stack | Might add premature complexity to MVP skeleton |
| A2 | Factory registry map is preferable to direct switch in this codebase | Architecture Patterns | Could over-engineer if provider count stays tiny |
| A3 | Monolithic-main pattern is objectively less current for this project size | State of the Art | Mostly maintainability/style impact |

## Open Questions (RESOLVED)

1. **Should Phase 1 include a `validate-config` CLI path?**
   - Decision: No. Phase 1 stays on a single `run` path so the first walking skeleton proves the real startup flow instead of introducing a second CLI surface.
   - Why: The context explicitly allows either choice, and the smaller correct slice is to share one validation path with actual startup rather than maintain a parallel command before runtime behavior exists.

2. **How strict should unknown-key handling be in TOML?**
   - Decision: Hard-fail on unknown keys across the root document and provider blocks.
   - Why: Phase 1 is locking a predictable operator-facing config contract. Silent ignores or warnings would undermine `CONF-02` by making misconfiguration look accepted while changing runtime behavior.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | Build/tests for Phase 1 | ✓ | go1.24.4 | Install/pin Go 1.26.3 via `mise` before implementation |
| Docker CLI/daemon | Later provider/integration checks; optional in this phase | ✓ | 29.4.1 | None needed for config-only unit tests |
| mise | Toolchain/version management policy | ✓ | 2026.5.4 | Manual installation if missing |
| Python + pip | slopcheck utility | ✓ | 3.13.7 | Skip legitimacy audit if unavailable |

**Missing dependencies with no fallback:**
- None blocking research; implementation should still upgrade Go toolchain to 1.26.x target.

**Missing dependencies with fallback:**
- Go 1.26.3 not currently installed; use `mise` pinning and local toolchain setup.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` package (stdlib) [CITED: /home/aaron/Code/docker-dns-sync/.planning/research/STACK.md] |
| Config file | none — rely on `go test` defaults in Phase 1 |
| Quick run command | `go test ./internal/config ./internal/runtime ./internal/state` |
| Full suite command | `go test ./...` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| CONF-01 | Requires 1+ sources and 1+ outputs | unit | `go test ./internal/config -run TestValidateRequiresSourceAndOutput` | ❌ Wave 0 |
| CONF-02 | Configurable state/log/retry/credential refs | unit | `go test ./internal/config -run TestValidateRuntimeAndCredentialFields` | ❌ Wave 0 |
| CONF-03 | Docker endpoint config accepts local socket or proxy | unit | `go test ./internal/config -run TestDockerEndpointModes` | ❌ Wave 0 |
| OPS-03 | New providers attach via factories without reconciler changes | unit | `go test ./internal/runtime -run TestFactoryRegistryExtensibility` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/config ./internal/runtime ./internal/state`
- **Per wave merge:** `go test ./...`
- **Phase gate:** Full suite green before `/gsd-verify-work`

### Wave 0 Gaps
- [ ] `internal/config/validate_test.go` — covers CONF-01/CONF-02/CONF-03
- [ ] `internal/runtime/factories_test.go` — covers OPS-03 extensibility seam
- [ ] `internal/state/atomic_file_test.go` — atomic write/rename behavior checks

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes | Secret references resolved from env; no plaintext logging |
| V3 Session Management | no | Not a session-based user app |
| V4 Access Control | yes | Restrict state-file permissions (`0600`) and least-privilege runtime user |
| V5 Input Validation | yes | Strict TOML semantic validation for startup invariants |
| V6 Cryptography | no | No custom cryptography in this phase |

### Known Threat Patterns for Go daemon runtime bootstrap

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Secret leakage via logs/errors | Information Disclosure | Redaction + never print resolved secret values |
| Malformed config causing unsafe defaults | Tampering | Fail-fast semantic validation with explicit required fields |
| State file tampering/permission drift | Tampering | Restrictive file mode + atomic writes + startup integrity checks |

## Sources

### Primary (HIGH confidence)
- `/home/aaron/Code/docker-dns-sync/.planning/phases/01-runtime-foundation-contracts/01-CONTEXT.md` - locked decisions and phase scope
- `/home/aaron/Code/docker-dns-sync/.planning/REQUIREMENTS.md` - CONF-01/02/03 and OPS-03 requirements
- `/home/aaron/Code/docker-dns-sync/.planning/ROADMAP.md` - phase goal/success criteria
- `/home/aaron/Code/docker-dns-sync/.planning/research/STACK.md` - standard stack guidance
- `/home/aaron/Code/docker-dns-sync/.planning/research/ARCHITECTURE.md` - runtime boundaries and contract shape
- `https://github.com/BurntSushi/toml` - TOML library documentation
- `https://docs.docker.com/reference/api/engine/sdk/examples/` - Docker SDK usage guidance

### Secondary (MEDIUM confidence)
- `/home/aaron/Code/docker-dns-sync/PRD.md` - architecture/security constraints and acceptance criteria details

### Tertiary (LOW confidence)
- None beyond assumptions explicitly listed.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - validated against project research docs and module version checks.
- Architecture: HIGH - directly constrained by locked phase decisions.
- Pitfalls: HIGH - aligned to existing project pitfall research and requirement invariants.

**Research date:** 2026-05-12
**Valid until:** 2026-06-11
