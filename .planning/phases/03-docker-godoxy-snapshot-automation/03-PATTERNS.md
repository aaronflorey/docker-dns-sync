# Phase 03: Docker/Godoxy Snapshot Automation - Pattern Map

**Mapped:** 2026-05-13
**Files analyzed:** 6
**Analogs found:** 6 / 6

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|---|---|---|---|---|
| `internal/providers/docker/source.go` | service | request-response + batch | `internal/providers/adguard/output.go` | role-match |
| `internal/providers/docker/labels.go` | utility | transform | `internal/runtime/reconcile_keys.go` | role-match |
| `internal/providers/docker/source_test.go` | test | batch | `internal/providers/adguard/output_test.go` | role-match |
| `internal/providers/docker/labels_test.go` | test | transform | `internal/runtime/reconcile_test.go` | role-match |
| `internal/runtime/app_test.go` | test | orchestration | `internal/runtime/app_test.go` | exact |
| `internal/runtime/factories_test.go` | test | batch | `internal/runtime/factories_test.go` | exact |

## Pattern Assignments

### `internal/providers/docker/source.go` (service, request-response + batch)

**Analog:** `internal/providers/adguard/output.go`

**Provider construction + narrow contract implementation**:
```go
func New(cfg config.SourceConfig) (*Provider, error) {
    cli, err := mobyclient.NewClientWithOpts(...)
    ...
    return &Provider{ref: contracts.ProviderRef{Type: cfg.Type, Name: cfg.Name}}, nil
}
```

**Use this pattern for:** preserving provider identity and endpoint-driven construction while replacing the `ListDesired` stub with real Docker snapshot behavior.

---

### `internal/providers/docker/labels.go` (utility, transform)

**Analog:** `internal/runtime/reconcile_keys.go`

**Small pure helpers for normalization/derivation**:
```go
func normalizeHost(value string) string { ... }
func desiredKey(record contracts.DesiredRecord) string { ... }
```

**Use this pattern for:** pure label parsing, alias expansion, supported wildcard/reference handling, and deterministic hostname sorting with minimal side effects.

---

### `internal/providers/docker/source_test.go` (test, batch)

**Analog:** `internal/providers/adguard/output_test.go`

**Table-driven provider contract tests**:
```go
func TestListDesired(t *testing.T) {
    t.Parallel()
    tests := []struct { ... }{ ... }
}
```

**Use this pattern for:** faking Docker client responses and asserting `[]contracts.DesiredRecord` output without involving runtime reconcile policy.

---

### `internal/providers/docker/labels_test.go` (test, transform)

**Analog:** `internal/runtime/reconcile_test.go`

**Focused subset coverage per behavior rule**:
```go
func TestAliasFallback(t *testing.T) { ... }
func TestExcludeSkipsContainer(t *testing.T) { ... }
```

**Use this pattern for:** freezing the exact Godoxy subset before source orchestration lands.

---

### `internal/runtime/app_test.go` (test, orchestration)

**Analog:** `internal/runtime/app_test.go`

**Startup reconcile ordering checks**:
```go
func TestAppRunExecutesStartupReconcileBeforeCancellation(t *testing.T) {
    ...
}
```

**Use this pattern for:** minimally extending startup assertions only if Phase 03 needs to prove the real Docker source plugs into the existing startup path.

---

### `internal/runtime/factories_test.go` (test, batch)

**Analog:** `internal/runtime/factories_test.go`

**Runtime registry behavior checks**:
```go
func TestDockerSourceUsesConfiguredEndpoint(t *testing.T) {
    ...
}
```

**Use this pattern for:** any narrow assertions that Docker provider wiring still preserves endpoint-driven construction after the source client interface widens for container listing.

## Cross-Cutting Implementation Notes

- Keep Docker-specific parsing inside `internal/providers/docker`; runtime should only see normalized `DesiredRecord` output.
- Prefer small fake Docker client interfaces in tests rather than spinning up a real Docker daemon during Phase 03 unit coverage.
- Sort aliases and final desired records before returning from `ListDesired` to keep the source deterministic.
- Reuse existing `contracts.SourceObjectRef` fields exactly: `Provider`, `ID`, and `DisplayName` should be sourced from Docker provider identity, container ID, and human-meaningful container name.
