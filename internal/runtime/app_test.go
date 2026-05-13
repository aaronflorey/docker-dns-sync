package runtime

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/aaronlmathis/docker-dns-sync/internal/config"
	"github.com/aaronlmathis/docker-dns-sync/internal/contracts"
)

func TestAppRunStartupReconcilesAndPersistsState(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "adguard", Name: "primary"}
	sourceRef := contracts.SourceObjectRef{Provider: contracts.ProviderRef{Type: "docker", Name: "local"}, ID: "ctr-1", DisplayName: "svc"}
	desired := contracts.DesiredRecord{Hostname: "app.local", Answer: "10.0.0.10", Source: sourceRef}

	app := New(testRuntimeConfig(filepath.Join(t.TempDir(), "state.json")))
	app.registry = testRegistry(
		stubSourceFactory{source: &startupSourceStub{provider: sourceRef.Provider, desired: []contracts.DesiredRecord{desired}}},
		stubOutputFactory{output: &startupOutputStub{provider: provider}},
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx)
	}()

	waitForCondition(t, func() bool {
		return app.store != nil && len(app.outputs) == 1
	})

	output := app.outputs[0].(*startupOutputStub)
	waitForCondition(t, func() bool {
		return len(output.created) == 1
	})

	snapshot, err := app.store.Load()
	if err != nil {
		t.Fatalf("load persisted snapshot: %v", err)
	}
	if len(snapshot.ManagedRecords) != 1 {
		t.Fatalf("expected one persisted managed record, got %d", len(snapshot.ManagedRecords))
	}
	if snapshot.ManagedRecords[0].Hostname != desired.Hostname || snapshot.ManagedRecords[0].Answer != desired.Answer {
		t.Fatalf("unexpected persisted record: %+v", snapshot.ManagedRecords[0])
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for shutdown")
	}
}

func TestAppRunExecutesStartupReconcileBeforeCancellation(t *testing.T) {
	t.Parallel()

	cfg := testRuntimeConfig(filepath.Join(t.TempDir(), "state.json"))
	ctx, cancel := context.WithCancel(context.Background())

	source := &startupSourceStub{
		provider: contracts.ProviderRef{Type: "docker", Name: "local"},
		onListDesired: func() {
			cancel()
		},
	}

	app := New(cfg)
	app.registry = testRegistry(
		stubSourceFactory{source: source},
		stubOutputFactory{output: &startupOutputStub{provider: contracts.ProviderRef{Type: "adguard", Name: "primary"}}},
	)

	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("startup reconcile did not run before timeout")
	}

	if source.listDesiredCalls != 1 {
		t.Fatalf("expected startup ListDesired call, got %d", source.listDesiredCalls)
	}
}

type startupSourceStub struct {
	provider         contracts.ProviderRef
	desired          []contracts.DesiredRecord
	listDesiredCalls int
	onListDesired    func()
}

func (s *startupSourceStub) Provider() contracts.ProviderRef {
	return s.provider
}

func (s *startupSourceStub) ListDesired(context.Context) ([]contracts.DesiredRecord, error) {
	s.listDesiredCalls++
	if s.onListDesired != nil {
		s.onListDesired()
	}
	return s.desired, nil
}

type startupOutputStub struct {
	provider         contracts.ProviderRef
	visible          []contracts.VisibleRecord
	created          []contracts.DesiredRecord
	listVisibleCalls int
}

func (o *startupOutputStub) Provider() contracts.ProviderRef {
	return o.provider
}

func (o *startupOutputStub) ListVisible(context.Context) ([]contracts.VisibleRecord, error) {
	o.listVisibleCalls++
	return o.visible, nil
}

func (o *startupOutputStub) Create(_ context.Context, desired contracts.DesiredRecord) error {
	o.created = append(o.created, desired)
	return nil
}

func (o *startupOutputStub) Update(context.Context, contracts.VisibleRecord, contracts.DesiredRecord) error {
	return nil
}

func (o *startupOutputStub) Delete(context.Context, contracts.VisibleRecord) error {
	return nil
}

type stubSourceFactory struct {
	source contracts.Source
}

func (f stubSourceFactory) build(config.SourceConfig, RuntimeDeps) (contracts.Source, error) {
	return f.source, nil
}

type stubOutputFactory struct {
	output contracts.Output
}

func (f stubOutputFactory) build(config.OutputConfig, RuntimeDeps) (contracts.Output, error) {
	return f.output, nil
}

func testRegistry(sourceFactory stubSourceFactory, outputFactory stubOutputFactory) *FactoryRegistry {
	registry := NewFactoryRegistry()
	mustRegister(registry.RegisterSource("docker", sourceFactory.build))
	mustRegister(registry.RegisterOutput("adguard", outputFactory.build))
	return registry
}

func testRuntimeConfig(statePath string) config.Config {
	return config.Config{
		Sources: []config.SourceConfig{{Type: "docker", Name: "local", Endpoint: "unix:///var/run/docker.sock"}},
		Outputs: []config.OutputConfig{{Type: "adguard", Name: "primary", URL: "http://127.0.0.1:3000", Username: "admin", Password: "secret"}},
		State:   config.StateConfig{Path: statePath},
		Logging: config.LoggingConfig{Level: "info", Format: "text"},
		Retry:   config.RetryConfig{InitialInterval: "1s", MaxInterval: "30s", MaxElapsedTime: "5m"},
	}
}

func waitForCondition(t *testing.T, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("condition was not met before timeout")
}
