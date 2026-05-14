# Phase 04: Recovery, Observability & Deployment - Pattern Map

**Mapped:** 2026-05-13
**Files analyzed:** 8
**Analogs found:** 8 / 8

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|---|---|---|---|---|
| `internal/runtime/app.go` | service | orchestration + loop | `internal/runtime/app.go` | exact |
| `internal/contracts/source.go` | interface | contract | `internal/contracts/source.go` | exact |
| `internal/providers/docker/source.go` | service | stream + batch | `internal/providers/docker/source.go` | exact |
| `internal/runtime/app_test.go` | test | orchestration | `internal/runtime/app_test.go` | exact |
| `internal/providers/docker/source_test.go` | test | stream + batch | `internal/providers/docker/source_test.go` | exact |
| `internal/runtime/deps.go` | utility | config -> runtime deps | `internal/runtime/deps.go` | exact |
| `README.md` | docs | operator guide | `PRD.md` | role-match |
| `deploy/systemd/docker-dns-sync.service` | ops artifact | process supervision | `cmd/docker-dns-sync/main.go` | role-match |

## Pattern Assignments

### `internal/runtime/app.go` (service, orchestration + loop)

**Analog:** `internal/runtime/app.go`

**Startup-first orchestration with explicit dependencies**:
```go
deps.Logger.Info("starting docker-dns-sync runtime", ...)
if err := a.startupReconcile(ctx); err != nil {
    return err
}
<-ctx.Done()
```

**Use this pattern for:** adding a steady-state loop after startup without introducing globals or a second runtime entrypoint.

---

### `internal/contracts/source.go` (interface, contract)

**Analog:** `internal/contracts/source.go`

**Small stable interfaces with explicit required methods**:
```go
type Source interface {
    Provider() ProviderRef
    ListDesired(context.Context) ([]DesiredRecord, error)
}
```

**Use this pattern for:** defining an optional watch-adjacent interface without bloating the base source contract.

---

### `internal/providers/docker/source.go` (service, stream + batch)

**Analog:** `internal/providers/docker/source.go`

**Provider-owned Docker client seam with minimal surface**:
```go
type apiClient interface {
    ContainerList(...)
}
```

**Use this pattern for:** widening the Docker client seam just enough for `Events` support while keeping Docker-specific details inside the provider package.

---

### `internal/runtime/app_test.go` (test, orchestration)

**Analog:** `internal/runtime/app_test.go`

**Cancellation-controlled runtime tests**:
```go
ctx, cancel := context.WithCancel(context.Background())
done := make(chan error, 1)
go func() { done <- app.Run(ctx) }()
```

**Use this pattern for:** proving startup, steady-state event handling, and reconnect/retry behavior without full integration infrastructure.

---

### `internal/providers/docker/source_test.go` (test, stream + batch)

**Analog:** `internal/providers/docker/source_test.go`

**Fake-client provider tests with deterministic events**:
```go
func TestListDesired(t *testing.T) { ... }
```

**Use this pattern for:** injecting fake Docker event streams and disconnects into the provider or watch seam.

---

### `internal/runtime/deps.go` (utility, config -> runtime deps)

**Analog:** `internal/runtime/deps.go`

**Parse-once runtime dependency setup**:
```go
return RuntimeDeps{Logger: slog.New(handler), Retry: retry}, nil
```

**Use this pattern for:** any retry helper or loop timing state that should remain config-driven and explicit.

---

### `README.md` and deployment artifacts (docs/ops)

**Analog:** `PRD.md` for operator-facing prose and `cmd/docker-dns-sync/main.go` for executable contract.

**Use this pattern for:** ensuring examples call the real binary contract (`docker-dns-sync -config ...`) and document mounted state/socket paths directly.

## Cross-Cutting Implementation Notes

- Prefer one reconcile helper that both startup and steady-state code call.
- Keep Docker-specific event filtering in `internal/providers/docker`; runtime should receive generic triggers or a narrow watch abstraction.
- Add logs at operation boundaries, not inside every tiny helper.
- Deployment examples should use the exact config keys already implemented in `internal/config/model.go`.
