# Phase 02: Ownership-Safe Reconciliation Core - Pattern Map

**Mapped:** 2026-05-13
**Files analyzed:** 7
**Analogs found:** 7 / 7

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|---|---|---|---|---|
| `internal/runtime/reconcile.go` | controller | batch/transform | `internal/runtime/app.go` | role-match |
| `internal/runtime/reconcile_plan.go` | utility | batch/transform | `internal/runtime/factories.go` | role-match |
| `internal/runtime/reconcile_apply.go` | service | request-response | `internal/state/store.go` | role-match |
| `internal/runtime/reconcile_keys.go` | utility | transform | `internal/config/secrets.go` | role-match |
| `internal/runtime/reconcile_errors.go` | utility | request-response | `internal/config/validate.go` | role-match |
| `internal/runtime/reconcile_test.go` | test | batch | `internal/runtime/factories_test.go` | role-match |
| `internal/runtime/reconcile_state_test.go` | test | file-I/O | `internal/state/atomic_file_test.go` | role-match |
| `internal/providers/adguard/output.go` | service | request-response | `internal/providers/adguardstub/output.go` | role-match |
| `internal/providers/adguard/output_test.go` | test | request-response | `internal/runtime/factories_test.go` | role-match |
| `internal/runtime/factories.go` | utility | batch/transform | `internal/runtime/factories.go` | exact |
| `internal/runtime/factories_test.go` | test | batch | `internal/runtime/factories_test.go` | exact |

## Pattern Assignments

### `internal/providers/adguard/output.go` (service, request-response)

**Analog:** `internal/providers/adguardstub/output.go`

**Provider identity + config mapping** (lines 14-32):
```go
func New(cfg config.OutputConfig) (*Output, error) {
	if strings.TrimSpace(cfg.Name) == "" {
		return nil, errors.New("output name is required")
	}
	...
	return &Output{
		provider: contracts.ProviderRef{Type: cfg.Type, Name: cfg.Name},
		url:      cfg.URL,
	}, nil
}
```

**Use this pattern for:** preserving `contracts.ProviderRef` shape and config-driven construction while replacing stub behavior with the real AdGuard HTTP transport.

---

### `internal/providers/adguard/output_test.go` (test, request-response)

**Analog:** `internal/runtime/factories_test.go`

**Parallel, table-driven test layout** (lines 13-44):
```go
func TestFactoryRegistryExtensibility(t *testing.T) {
	t.Parallel()
	...
}
```

**Use this pattern for:** small HTTP-backed contract tests that assert request shape, decoded results, and error behavior without embedding reconcile policy.

---

### `internal/runtime/factories.go` (utility, batch/transform)

**Analog:** `internal/runtime/factories.go`

**Registry mutation + duplicate guard** (lines 29-75):
```go
func (r *FactoryRegistry) RegisterOutput(providerType string, factory OutputFactory) error {
	if factory == nil {
		return fmt.Errorf("output factory for %q is required", providerType)
	}
	if _, exists := r.outputFactories[providerType]; exists {
		return fmt.Errorf("output provider type %q is already registered", providerType)
	}
	...
}
```

**Use this pattern for:** narrow factory wiring updates that swap the concrete AdGuard provider without changing runtime/provider responsibilities.

---

### `internal/runtime/factories_test.go` (test, batch)

**Analog:** `internal/runtime/factories_test.go`

**Registry assertions** (lines 46-101):
```go
func TestBuildProvidersFromConfig(t *testing.T) {
	t.Parallel()
	...
}
```

**Use this pattern for:** asserting the default registry now returns the real AdGuard provider while keeping provider registration and construction behavior stable.

---

### `internal/runtime/reconcile.go` (controller, batch/transform)

**Analog:** `internal/runtime/app.go`

**Imports + orchestration setup** (lines 3-20):
```go
import (
	"context"
	"log/slog"
	"os"

	"github.com/aaronlmathis/docker-dns-sync/internal/config"
	"github.com/aaronlmathis/docker-dns-sync/internal/contracts"
	"github.com/aaronlmathis/docker-dns-sync/internal/state"
)
```

**Runtime bootstrap / ownership of state + providers** (lines 29-63):
```go
func (a *App) Run(ctx context.Context) error {
	if err := config.Validate(a.cfg); err != nil {
		return err
	}

	resolved, err := config.ResolveSecrets(a.cfg, os.LookupEnv)
	...
	store, err := state.NewStore(resolved.State.Path)
	...
	sources, outputs, err := a.registry.BuildProviders(resolved, deps)
	...
	deps.Logger.Info("starting docker-dns-sync runtime", "sources", len(sources), "outputs", len(outputs), "state_path", resolved.State.Path)
	<-ctx.Done()
	deps.Logger.Info("runtime cancelled", "reason", ctx.Err())
	return nil
}
```

**Use this pattern for:** one runtime-level reconcile entrypoint that owns the context, reads desired/visible/owned inputs, and keeps provider logic out of the orchestration layer.

---

### `internal/runtime/reconcile_plan.go` (utility, batch/transform)

**Analog:** `internal/runtime/factories.go`

**Indexed batch processing + early return errors** (lines 77-126):
```go
func (r *FactoryRegistry) BuildProviders(cfg config.Config, deps RuntimeDeps) ([]contracts.Source, []contracts.Output, error) {
	sources, err := r.BuildSources(cfg, deps)
	if err != nil {
		return nil, nil, err
	}

	outputs, err := r.BuildOutputs(cfg, deps)
	if err != nil {
		return nil, nil, err
	}

	return sources, outputs, nil
}

func (r *FactoryRegistry) BuildSources(cfg config.Config, deps RuntimeDeps) ([]contracts.Source, error) {
	for i, sourceCfg := range cfg.Sources {
		factory, ok := r.sourceFactories[sourceCfg.Type]
		if !ok {
			return nil, fmt.Errorf("sources[%d]: unknown provider type %q", i, sourceCfg.Type)
		}
		...
	}
}
```

**Indexed validation style** (lines 19-60) from `internal/config/validate.go`:
```go
for i, source := range cfg.Sources {
	if strings.TrimSpace(source.Type) == "" {
		return fmt.Errorf("sources[%d].type is required", i)
	}
	...
}
```

**Use this pattern for:** deterministic key indexing and diff planning with explicit per-item errors instead of hidden collection-level behavior.

---

### `internal/runtime/reconcile_apply.go` (service, request-response)

**Analog:** `internal/state/store.go`

**Read/validate/normalize snapshot before use** (lines 40-59):
```go
func (s *Store) Load() (Snapshot, error) {
	payload, err := os.ReadFile(s.path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read state file: %w", err)
	}

	var snapshot Snapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("decode state file: %w", err)
	}

	if snapshot.Version != SnapshotVersion {
		return Snapshot{}, fmt.Errorf("unsupported state snapshot version %d", snapshot.Version)
	}
	...
}
```

**Persist only after success, with explicit traceability fields** (lines 62-79 + `internal/state/model.go` lines 11-27):
```go
func (s *Store) Save(snapshot Snapshot) error {
	snapshot.Version = SnapshotVersion
	if snapshot.ManagedRecords == nil {
		snapshot.ManagedRecords = []ManagedRecord{}
	}

	payload, err := json.MarshalIndent(snapshot, "", "  ")
	...
	if err := atomicWriteFile(s.path, payload, 0o600); err != nil {
		return err
	}
	return nil
}
```

```go
type ManagedRecord struct {
	Output        contracts.ProviderRef     `json:"output"`
	Source        contracts.SourceObjectRef `json:"source"`
	Hostname      string                    `json:"hostname"`
	Answer        string                    `json:"answer"`
	LastAppliedAt time.Time                 `json:"last_applied_at"`
}
```

**Atomic file write order** (`internal/state/atomic_file.go`, lines 9-52):
```go
tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
...
if err := tmp.Sync(); err != nil { ... }
if err := tmp.Close(); err != nil { ... }
if err := os.Rename(tmpPath, path); err != nil { ... }
```

**Use this pattern for:** apply-then-persist sequencing, state snapshot rewrites, and `LastAppliedAt` updates derived from successful remote results only.

---

### `internal/runtime/reconcile_keys.go` (utility, transform)

**Analog:** `internal/config/secrets.go`

**Small pure parser / normalizer helpers** (lines 13-55):
```go
func ResolveSecrets(cfg Config, lookup LookupEnvFunc) (Config, error) {
	if lookup == nil {
		return Config{}, errors.New("secret lookup is required")
	}
	...
	ref := strings.TrimSpace(resolved.Outputs[i].PasswordRef)
	if ref == "" {
		continue
	}
	...
	name, err := parseEnvRef(ref)
	...
}
```

```go
func parseEnvRef(ref string) (string, error) {
	if !strings.HasPrefix(ref, envRefPrefix) {
		return "", fmt.Errorf("must start with %q", envRefPrefix)
	}
	name := strings.TrimSpace(strings.TrimPrefix(ref, envRefPrefix))
	if name == "" {
		return "", errors.New("must include an environment variable name")
	}
	return name, nil
}
```

**Use this pattern for:** canonical `(hostname, answer)` key helpers that trim/normalize inputs and return explicit errors on malformed values.

---

### `internal/runtime/reconcile_errors.go` (utility, request-response)

**Analog:** `internal/config/validate.go`

**Plain error strings for simple safety checks** (lines 10-18):
```go
func Validate(cfg Config) error {
	if len(cfg.Sources) == 0 {
		return errors.New("at least one source must be configured")
	}
	if len(cfg.Outputs) == 0 {
		return errors.New("at least one output must be configured")
	}
```

**Wrapped, indexed, field-specific errors** (lines 19-111):
```go
if strings.TrimSpace(output.URL) == "" {
	return fmt.Errorf("outputs[%d].url is required", i)
}
...
if duration <= 0 {
	return fmt.Errorf("%s must be greater than zero", field)
}
```

**Also use provider-style wrapping** from `internal/runtime/factories.go` (lines 41-75):
```go
if factory == nil {
	return fmt.Errorf("source factory for %q is required", providerType)
}
if _, exists := r.sourceFactories[providerType]; exists {
	return fmt.Errorf("source provider type %q is already registered", providerType)
}
```

**Use this pattern for:** typed ambiguity/safety errors that stay human-readable, index-aware, and free of framework dependencies.

---

### `internal/runtime/reconcile_test.go` (test, batch)

**Analog:** `internal/runtime/factories_test.go`

**Parallel tests + fake contracts** (lines 13-44, 102-136):
```go
func TestFactoryRegistryExtensibility(t *testing.T) {
	t.Parallel()
    ...
}
```

```go
type fakeSource struct {
	provider contracts.ProviderRef
}

func (fakeSource) ListDesired(context.Context) ([]contracts.DesiredRecord, error) {
	return nil, nil
}

type fakeOutput struct {
	provider contracts.ProviderRef
}

func (fakeOutput) ListVisible(context.Context) ([]contracts.VisibleRecord, error) {
	return nil, nil
}
func (fakeOutput) Create(context.Context, contracts.DesiredRecord) error { return nil }
func (fakeOutput) Update(context.Context, contracts.VisibleRecord, contracts.DesiredRecord) error { return nil }
func (fakeOutput) Delete(context.Context, contracts.VisibleRecord) error { return nil }
```

**Table-driven assertion style** from `internal/config/validate_test.go` (lines 59-121):
```go
tests := []struct {
	name    string
	mutate  func(*Config)
	wantErr string
}{
	{ name: "missing state path", ... },
}
```

**Use this pattern for:** reconcile plan/apply tests with fake source/output/state adapters and explicit failure cases for ownership gating and duplicate visible records.

---

### `internal/runtime/reconcile_state_test.go` (test, file-I/O)

**Analog:** `internal/state/atomic_file_test.go`

**Temp dir + replace-in-place assertion** (lines 9-39):
```go
dir := t.TempDir()
path := filepath.Join(dir, "ownership.json")
if err := os.WriteFile(path, []byte("old-state\n"), 0o600); err != nil {
	t.Fatalf("seed state file: %v", err)
}

if err := atomicWriteFile(path, []byte("new-state\n"), 0o600); err != nil {
	t.Fatalf("atomic write: %v", err)
}
...
if string(payload) != "new-state\n" {
	t.Fatalf("expected replaced state contents, got %q", string(payload))
}
```

**Use this pattern for:** state snapshot ordering tests that prove writes happen only after successful apply and that the final snapshot on disk is the observed result.

## Shared Patterns

### Runtime owns orchestration, providers stay narrow
**Source:** `internal/runtime/app.go`, `internal/runtime/factories.go`

```go
resolved, err := config.ResolveSecrets(a.cfg, os.LookupEnv)
...
store, err := state.NewStore(resolved.State.Path)
...
sources, outputs, err := a.registry.BuildProviders(resolved, deps)
```

Apply to: `internal/runtime/reconcile.go`, `internal/runtime/reconcile_plan.go`, `internal/runtime/reconcile_apply.go`

### Atomic state persistence
**Source:** `internal/state/store.go`, `internal/state/atomic_file.go`

```go
snapshot.Version = SnapshotVersion
if snapshot.ManagedRecords == nil {
	snapshot.ManagedRecords = []ManagedRecord{}
}
...
if err := os.Rename(tmpPath, path); err != nil {
	return fmt.Errorf("rename temp state file: %w", err)
}
```

Apply to: `internal/runtime/reconcile_apply.go`, `internal/runtime/reconcile_state_test.go`

### Indexed, explicit errors
**Source:** `internal/config/validate.go`, `internal/runtime/factories.go`

```go
return fmt.Errorf("outputs[%d].username is required", i)
return fmt.Errorf("sources[%d]: unknown provider type %q", i, sourceCfg.Type)
```

Apply to: `internal/runtime/reconcile_errors.go`, `internal/runtime/reconcile_plan.go`

### Table-driven tests with fakes
**Source:** `internal/config/validate_test.go`, `internal/runtime/factories_test.go`

```go
t.Parallel()
tests := []struct { ... }{ ... }
```

Apply to: `internal/runtime/reconcile_test.go`, `internal/runtime/reconcile_state_test.go`

## Metadata

**Analog search scope:** `internal/runtime`, `internal/state`, `internal/contracts`, `internal/providers`, `internal/config`, `cmd/docker-dns-sync`
**Files scanned:** 17
**Pattern extraction date:** 2026-05-13
