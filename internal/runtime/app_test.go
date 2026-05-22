package runtime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aaronflorey/docker-dns-sync/internal/config"
	"github.com/aaronflorey/docker-dns-sync/internal/contracts"
	"github.com/aaronflorey/docker-dns-sync/internal/state"
)

func TestAppRunStartupReconcilesAndPersistsState(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "adguard", Name: "primary"}
	sourceRef := contracts.SourceObjectRef{Provider: contracts.ProviderRef{Type: "docker", Name: "local"}, ID: "ctr-1", DisplayName: "svc"}
	desired := contracts.DesiredRecord{Hostname: "app.local", Answer: "10.0.0.10", Source: sourceRef}
	output := &startupOutputStub{provider: provider}
	statePath := filepath.Join(t.TempDir(), "state.json")

	app := New(testRuntimeConfig(statePath))
	app.registry = testRegistry(
		stubSourceFactory{source: &startupSourceStub{provider: sourceRef.Provider, desired: []contracts.DesiredRecord{desired}}},
		stubOutputFactory{output: output},
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx)
	}()

	waitForCondition(t, func() bool {
		return output.createCount() == 1
	})

	store, err := state.NewStore(statePath)
	if err != nil {
		t.Fatalf("create verification store: %v", err)
	}

	snapshot, err := store.Load()
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

	if source.listDesiredCallCount() != 1 {
		t.Fatalf("expected startup ListDesired call, got %d", source.listDesiredCallCount())
	}
}

func TestAppRunRuntimeReconcilesAfterWatchHint(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}
	session := newWatchSession()
	source := &watchSourceStub{
		startupSourceStub: startupSourceStub{
			provider: provider,
			desired: []contracts.DesiredRecord{{
				Hostname: "app.local",
				Answer:   "10.0.0.10",
				Source:   contracts.SourceObjectRef{Provider: provider, ID: "ctr-1", DisplayName: "svc"},
			}},
		},
		sessions: []*watchSession{session},
	}
	output := &startupOutputStub{provider: contracts.ProviderRef{Type: "adguard", Name: "primary"}}

	app := New(testRuntimeConfig(filepath.Join(t.TempDir(), "state.json")))
	app.registry = testRegistry(stubSourceFactory{source: source}, stubOutputFactory{output: output})
	app.newDeps = func(config.Config) (RuntimeDeps, error) {
		return RuntimeDeps{
			Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
			Retry: RetryPolicy{
				InitialInterval: 10 * time.Millisecond,
				MaxInterval:     20 * time.Millisecond,
				MaxElapsedTime:  200 * time.Millisecond,
			},
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	waitForCondition(t, func() bool {
		return source.listDesiredCallCount() == 2 && output.createCount() == 1 && output.listVisibleCount() == 2
	})

	session.hints <- struct{}{}
	waitForCondition(t, func() bool {
		return source.listDesiredCallCount() >= 3 && output.listVisibleCount() >= 3
	})

	cancel()
	assertRunStops(t, done)
}

func TestAppRunReconnectsAfterWatchDisconnect(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}
	first := newWatchSession()
	second := newWatchSession()
	source := &watchSourceStub{
		startupSourceStub: startupSourceStub{
			provider: provider,
			desired: []contracts.DesiredRecord{{
				Hostname: "app.local",
				Answer:   "10.0.0.10",
				Source:   contracts.SourceObjectRef{Provider: provider, ID: "ctr-1", DisplayName: "svc"},
			}},
		},
		sessions: []*watchSession{first, second},
	}
	output := &startupOutputStub{provider: contracts.ProviderRef{Type: "adguard", Name: "primary"}}

	app := New(testRuntimeConfig(filepath.Join(t.TempDir(), "state.json")))
	app.registry = testRegistry(stubSourceFactory{source: source}, stubOutputFactory{output: output})
	app.newDeps = func(config.Config) (RuntimeDeps, error) {
		return RuntimeDeps{
			Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
			Retry: RetryPolicy{
				InitialInterval: 10 * time.Millisecond,
				MaxInterval:     20 * time.Millisecond,
				MaxElapsedTime:  200 * time.Millisecond,
			},
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	waitForCondition(t, func() bool {
		return source.listDesiredCallCount() == 2 && source.watchCallCount() == 1
	})

	first.errs <- io.EOF
	waitForCondition(t, func() bool {
		return source.listDesiredCallCount() >= 3 && source.watchCallCount() >= 2
	})

	second.hints <- struct{}{}
	waitForCondition(t, func() bool {
		return source.listDesiredCallCount() >= 4
	})

	cancel()
	assertRunStops(t, done)
}

func TestAppRunRetriesReconnectRepairAfterTransientSourceFailure(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}
	first := newWatchSession()
	second := newWatchSession()
	source := &watchSourceStub{
		startupSourceStub: startupSourceStub{
			provider:             provider,
			failListDesiredAfter: 2,
			failListDesired:      2,
			desired: []contracts.DesiredRecord{{
				Hostname: "app.local",
				Answer:   "10.0.0.10",
				Source:   contracts.SourceObjectRef{Provider: provider, ID: "ctr-1", DisplayName: "svc"},
			}},
		},
		sessions: []*watchSession{first, second},
	}
	output := &startupOutputStub{provider: contracts.ProviderRef{Type: "adguard", Name: "primary"}}

	app := New(testRuntimeConfig(filepath.Join(t.TempDir(), "state.json")))
	app.registry = testRegistry(stubSourceFactory{source: source}, stubOutputFactory{output: output})
	app.newDeps = func(config.Config) (RuntimeDeps, error) {
		return RuntimeDeps{
			Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
			Retry: RetryPolicy{
				InitialInterval: time.Millisecond,
				MaxInterval:     2 * time.Millisecond,
				MaxElapsedTime:  50 * time.Millisecond,
			},
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	waitForCondition(t, func() bool {
		return source.listDesiredCallCount() == 2 && source.watchCallCount() == 1
	})

	first.errs <- io.EOF
	waitForCondition(t, func() bool {
		return source.listDesiredCallCount() >= 5 && source.watchCallCount() >= 2
	})
	assertRunStillRunning(t, done)

	second.hints <- struct{}{}
	waitForCondition(t, func() bool {
		return source.listDesiredCallCount() >= 6
	})

	cancel()
	assertRunStops(t, done)
}

func TestAppRunRetriesWatchHintReconcileAfterTransientSourceFailure(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}
	session := newWatchSession()
	source := &watchSourceStub{
		startupSourceStub: startupSourceStub{
			provider:             provider,
			failListDesiredAfter: 2,
			failListDesired:      2,
			desired: []contracts.DesiredRecord{{
				Hostname: "app.local",
				Answer:   "10.0.0.10",
				Source:   contracts.SourceObjectRef{Provider: provider, ID: "ctr-1", DisplayName: "svc"},
			}},
		},
		sessions: []*watchSession{session},
	}
	output := &startupOutputStub{provider: contracts.ProviderRef{Type: "adguard", Name: "primary"}}

	app := New(testRuntimeConfig(filepath.Join(t.TempDir(), "state.json")))
	app.registry = testRegistry(stubSourceFactory{source: source}, stubOutputFactory{output: output})
	app.newDeps = func(config.Config) (RuntimeDeps, error) {
		return RuntimeDeps{
			Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
			Retry: RetryPolicy{
				InitialInterval: time.Millisecond,
				MaxInterval:     2 * time.Millisecond,
				MaxElapsedTime:  50 * time.Millisecond,
			},
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	waitForCondition(t, func() bool {
		return source.listDesiredCallCount() == 2 && output.createCount() == 1
	})

	session.hints <- struct{}{}
	waitForCondition(t, func() bool {
		return source.listDesiredCallCount() >= 5 && output.listVisibleCount() >= 3
	})
	assertRunStillRunning(t, done)

	cancel()
	assertRunStops(t, done)
}

func TestAppRunRetriesStartupHandoffReconcileAfterTransientSourceFailure(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}
	session := newWatchSession()
	source := &watchSourceStub{
		startupSourceStub: startupSourceStub{
			provider:             provider,
			failListDesiredAfter: 1,
			failListDesired:      2,
			desired: []contracts.DesiredRecord{{
				Hostname: "app.local",
				Answer:   "10.0.0.10",
				Source:   contracts.SourceObjectRef{Provider: provider, ID: "ctr-1", DisplayName: "svc"},
			}},
		},
		sessions: []*watchSession{session},
	}
	output := &startupOutputStub{provider: contracts.ProviderRef{Type: "adguard", Name: "primary"}}

	app := New(testRuntimeConfig(filepath.Join(t.TempDir(), "state.json")))
	app.registry = testRegistry(stubSourceFactory{source: source}, stubOutputFactory{output: output})
	app.newDeps = func(config.Config) (RuntimeDeps, error) {
		return RuntimeDeps{
			Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
			Retry: RetryPolicy{
				InitialInterval: time.Millisecond,
				MaxInterval:     2 * time.Millisecond,
				MaxElapsedTime:  50 * time.Millisecond,
			},
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	waitForCondition(t, func() bool {
		return source.watchCallCount() == 1 && source.listDesiredCallCount() >= 4 && output.listVisibleCount() >= 2
	})
	assertRunStillRunning(t, done)

	cancel()
	assertRunStops(t, done)
}

func TestAppRunRetriesWatchTriggeredReconcileAfterOtherSourceFailure(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "state.json")
	store, err := state.NewStore(statePath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	watchProvider := contracts.ProviderRef{Type: "docker", Name: "watch"}
	otherProvider := contracts.ProviderRef{Type: "docker", Name: "peer"}
	session := newWatchSession()
	watchSource := &watchSourceStub{
		startupSourceStub: startupSourceStub{
			provider: watchProvider,
			desired: []contracts.DesiredRecord{{
				Hostname: "watch.local",
				Answer:   "10.0.0.10",
				Source:   contracts.SourceObjectRef{Provider: watchProvider, ID: "ctr-watch", DisplayName: "watch"},
			}},
		},
		sessions: []*watchSession{session},
	}
	otherSource := &startupSourceStub{
		provider:             otherProvider,
		failListDesiredAfter: 2,
		failListDesired:      1,
		desired: []contracts.DesiredRecord{{
			Hostname: "peer.local",
			Answer:   "10.0.0.11",
			Source:   contracts.SourceObjectRef{Provider: otherProvider, ID: "ctr-peer", DisplayName: "peer"},
		}},
	}
	output := &startupOutputStub{provider: contracts.ProviderRef{Type: "adguard", Name: "primary"}}
	deps := RuntimeDeps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
		Retry: RetryPolicy{
			InitialInterval: time.Millisecond,
			MaxInterval:     2 * time.Millisecond,
			MaxElapsedTime:  50 * time.Millisecond,
		},
	}

	app := New(testRuntimeConfig(statePath))
	app.deps = deps
	app.store = store
	app.sources = []contracts.Source{watchSource, otherSource}
	app.outputs = wrapOutputs([]contracts.Output{output}, deps)

	if err := app.reconcile(context.Background(), "startup"); err != nil {
		t.Fatalf("startup reconcile: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.runSteadyState(ctx) }()

	waitForCondition(t, func() bool {
		return watchSource.watchCallCount() == 1 && watchSource.listDesiredCallCount() == 2 && otherSource.listDesiredCallCount() == 2
	})

	session.hints <- struct{}{}
	waitForCondition(t, func() bool {
		return watchSource.listDesiredCallCount() >= 4 && otherSource.listDesiredCallCount() >= 4 && output.listVisibleCount() >= 3
	})
	assertRunStillRunning(t, done)

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("steady state returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for steady-state shutdown")
	}
}

func TestAppRunDoesNotRetryWatchHintReconcileAfterMutationFailure(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}
	session := newWatchSession()
	source := &watchSourceStub{
		startupSourceStub: startupSourceStub{provider: provider},
		sessions:          []*watchSession{session},
	}
	output := &transientOutputStub{
		provider:        contracts.ProviderRef{Type: "adguard", Name: "primary"},
		failCreate:      1,
		failWithMessage: "adguard unavailable",
	}

	app := New(testRuntimeConfig(filepath.Join(t.TempDir(), "state.json")))
	app.registry = testRegistry(stubSourceFactory{source: source}, stubOutputFactory{output: output})
	app.newDeps = func(config.Config) (RuntimeDeps, error) {
		return RuntimeDeps{
			Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
			Retry: RetryPolicy{
				InitialInterval: time.Millisecond,
				MaxInterval:     2 * time.Millisecond,
				MaxElapsedTime:  50 * time.Millisecond,
			},
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	waitForCondition(t, func() bool {
		return source.listDesiredCallCount() == 2 && output.listVisibleCount() == 2
	})

	source.mu.Lock()
	source.desired = []contracts.DesiredRecord{{
		Hostname: "app.local",
		Answer:   "10.0.0.10",
		Source:   contracts.SourceObjectRef{Provider: provider, ID: "ctr-1", DisplayName: "svc"},
	}}
	source.mu.Unlock()

	session.hints <- struct{}{}

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected runtime error")
		}
		if !strings.Contains(err.Error(), "adguard unavailable") {
			t.Fatalf("expected mutation failure, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for mutation failure")
	}

	if output.createCount() != 1 {
		t.Fatalf("expected 1 Create call, got %d", output.createCount())
	}
	if source.listDesiredCallCount() != 3 {
		t.Fatalf("expected single watch-hint reconcile attempt after startup handoff resync, got %d ListDesired calls", source.listDesiredCallCount())
	}
}

func TestAppRunResyncsAfterStartingWatches(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}
	session := newWatchSession()
	source := &watchSourceStub{
		startupSourceStub: startupSourceStub{
			provider: provider,
			desired: []contracts.DesiredRecord{{
				Hostname: "app.local",
				Answer:   "10.0.0.10",
				Source:   contracts.SourceObjectRef{Provider: provider, ID: "ctr-1", DisplayName: "svc"},
			}},
		},
		sessions: []*watchSession{session},
	}
	output := &startupOutputStub{provider: contracts.ProviderRef{Type: "adguard", Name: "primary"}}

	app := New(testRuntimeConfig(filepath.Join(t.TempDir(), "state.json")))
	app.registry = testRegistry(stubSourceFactory{source: source}, stubOutputFactory{output: output})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	waitForCondition(t, func() bool {
		return source.listDesiredCallCount() == 2 && source.watchCallCount() == 1 && output.createCount() == 1 && output.listVisibleCount() == 2
	})

	cancel()
	assertRunStops(t, done)
}

func TestAppRunTreatsCleanWatchClosureAsDisconnect(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}
	first := newWatchSession()
	second := newWatchSession()
	source := &watchSourceStub{
		startupSourceStub: startupSourceStub{
			provider: provider,
			desired: []contracts.DesiredRecord{{
				Hostname: "app.local",
				Answer:   "10.0.0.10",
				Source:   contracts.SourceObjectRef{Provider: provider, ID: "ctr-1", DisplayName: "svc"},
			}},
		},
		sessions: []*watchSession{first, second},
	}
	output := &startupOutputStub{provider: contracts.ProviderRef{Type: "adguard", Name: "primary"}}

	app := New(testRuntimeConfig(filepath.Join(t.TempDir(), "state.json")))
	app.registry = testRegistry(stubSourceFactory{source: source}, stubOutputFactory{output: output})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	waitForCondition(t, func() bool {
		return source.listDesiredCallCount() == 2 && source.watchCallCount() == 1
	})

	close(first.hints)
	close(first.errs)
	waitForCondition(t, func() bool {
		return source.listDesiredCallCount() >= 3 && source.watchCallCount() >= 2
	})

	cancel()
	assertRunStops(t, done)
}

func TestAppRunRestartsWatchBeforeReconnectRepairRetries(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}
	first := newWatchSession()
	second := newWatchSession()
	source := &watchSourceStub{
		startupSourceStub: startupSourceStub{
			provider:             provider,
			failListDesiredAfter: 2,
			failListDesired:      3,
			desired: []contracts.DesiredRecord{{
				Hostname: "app.local",
				Answer:   "10.0.0.10",
				Source:   contracts.SourceObjectRef{Provider: provider, ID: "ctr-1", DisplayName: "svc"},
			}},
		},
		sessions: []*watchSession{first, second},
	}
	output := &startupOutputStub{provider: contracts.ProviderRef{Type: "adguard", Name: "primary"}}

	app := New(testRuntimeConfig(filepath.Join(t.TempDir(), "state.json")))
	app.registry = testRegistry(stubSourceFactory{source: source}, stubOutputFactory{output: output})
	app.newDeps = func(config.Config) (RuntimeDeps, error) {
		return RuntimeDeps{
			Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
			Retry: RetryPolicy{
				InitialInterval: 10 * time.Millisecond,
				MaxInterval:     10 * time.Millisecond,
				MaxElapsedTime:  200 * time.Millisecond,
			},
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	waitForCondition(t, func() bool {
		return source.listDesiredCallCount() == 2 && source.watchCallCount() == 1
	})

	first.errs <- io.EOF
	waitForConditionWithin(t, 100*time.Millisecond, func() bool {
		return source.watchCallCount() >= 2
	})
	assertRunStillRunning(t, done)

	cancel()
	assertRunStops(t, done)
}

func TestAppRunBacksOffRepeatedWatchReconnects(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}
	first := newWatchSession()
	second := newWatchSession()
	third := newWatchSession()
	source := &watchSourceStub{
		startupSourceStub: startupSourceStub{
			provider: provider,
			desired: []contracts.DesiredRecord{{
				Hostname: "app.local",
				Answer:   "10.0.0.10",
				Source:   contracts.SourceObjectRef{Provider: provider, ID: "ctr-1", DisplayName: "svc"},
			}},
		},
		sessions: []*watchSession{first, second, third},
	}
	output := &startupOutputStub{provider: contracts.ProviderRef{Type: "adguard", Name: "primary"}}

	app := New(testRuntimeConfig(filepath.Join(t.TempDir(), "state.json")))
	app.registry = testRegistry(stubSourceFactory{source: source}, stubOutputFactory{output: output})
	app.newDeps = func(config.Config) (RuntimeDeps, error) {
		return RuntimeDeps{
			Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
			Retry: RetryPolicy{
				InitialInterval: 10 * time.Millisecond,
				MaxInterval:     20 * time.Millisecond,
				MaxElapsedTime:  200 * time.Millisecond,
			},
		}, nil
	}
	app.newDeps = func(config.Config) (RuntimeDeps, error) {
		return RuntimeDeps{
			Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
			Retry: RetryPolicy{
				InitialInterval: 25 * time.Millisecond,
				MaxInterval:     50 * time.Millisecond,
				MaxElapsedTime:  200 * time.Millisecond,
			},
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	waitForCondition(t, func() bool {
		return source.listDesiredCallCount() == 2 && source.watchCallCount() == 1
	})

	first.errs <- io.EOF
	waitForConditionWithin(t, 100*time.Millisecond, func() bool {
		return source.watchCallCount() >= 2
	})

	second.errs <- io.EOF
	assertConditionHolds(t, 20*time.Millisecond, func() bool { return source.watchCallCount() == 2 })
	waitForConditionWithin(t, 100*time.Millisecond, func() bool {
		return source.watchCallCount() >= 3
	})
	assertRunStillRunning(t, done)

	cancel()
	assertRunStops(t, done)
}

func TestAppRunReconnectBackoffDoesNotBlockOtherWatchHints(t *testing.T) {
	t.Parallel()

	firstProvider := contracts.ProviderRef{Type: "docker", Name: "first"}
	secondProvider := contracts.ProviderRef{Type: "docker", Name: "second"}
	firstSession := newWatchSession()
	secondSession := newWatchSession()
	peerSession := newWatchSession()
	firstSource := &watchSourceStub{
		startupSourceStub: startupSourceStub{
			provider: firstProvider,
			desired: []contracts.DesiredRecord{{
				Hostname: "first.local",
				Answer:   "10.0.0.10",
				Source:   contracts.SourceObjectRef{Provider: firstProvider, ID: "ctr-first", DisplayName: "first"},
			}},
		},
		sessions: []*watchSession{firstSession, secondSession},
	}
	secondSource := &watchSourceStub{
		startupSourceStub: startupSourceStub{
			provider: secondProvider,
			desired: []contracts.DesiredRecord{{
				Hostname: "second.local",
				Answer:   "10.0.0.11",
				Source:   contracts.SourceObjectRef{Provider: secondProvider, ID: "ctr-second", DisplayName: "second"},
			}},
		},
		sessions: []*watchSession{peerSession},
	}
	output := &startupOutputStub{provider: contracts.ProviderRef{Type: "adguard", Name: "primary"}}
	deps := RuntimeDeps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
		Retry: RetryPolicy{
			InitialInterval: 50 * time.Millisecond,
			MaxInterval:     50 * time.Millisecond,
			MaxElapsedTime:  200 * time.Millisecond,
		},
	}

	app := New(testRuntimeConfig(filepath.Join(t.TempDir(), "state.json")))
	app.deps = deps
	store, err := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	app.store = store
	app.sources = []contracts.Source{firstSource, secondSource}
	app.outputs = wrapOutputs([]contracts.Output{output}, deps)

	if err := app.reconcile(context.Background(), "startup"); err != nil {
		t.Fatalf("startup reconcile: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.runSteadyState(ctx) }()

	waitForCondition(t, func() bool {
		return firstSource.watchCallCount() == 1 && secondSource.watchCallCount() == 1 && firstSource.listDesiredCallCount() == 2 && secondSource.listDesiredCallCount() == 2
	})

	firstSession.errs <- io.EOF
	peerSession.hints <- struct{}{}

	waitForConditionWithin(t, 30*time.Millisecond, func() bool {
		return firstSource.listDesiredCallCount() >= 3 && secondSource.listDesiredCallCount() >= 3 && output.listVisibleCount() >= 3
	})
	if firstSource.watchCallCount() != 1 {
		t.Fatalf("expected reconnect backoff to defer first source restart, got %d watch calls", firstSource.watchCallCount())
	}

	waitForConditionWithin(t, 120*time.Millisecond, func() bool {
		return firstSource.watchCallCount() >= 2
	})
	assertRunStillRunning(t, done)

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("steady state returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for steady-state shutdown")
	}
}

func TestAppRunResetsReconnectBackoffAfterSuccessfulRepair(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}
	first := newWatchSession()
	second := newWatchSession()
	third := newWatchSession()
	source := &watchSourceStub{
		startupSourceStub: startupSourceStub{
			provider: provider,
			desired: []contracts.DesiredRecord{{
				Hostname: "app.local",
				Answer:   "10.0.0.10",
				Source:   contracts.SourceObjectRef{Provider: provider, ID: "ctr-1", DisplayName: "svc"},
			}},
		},
		sessions: []*watchSession{first, second, third},
	}
	output := &startupOutputStub{provider: contracts.ProviderRef{Type: "adguard", Name: "primary"}}

	app := New(testRuntimeConfig(filepath.Join(t.TempDir(), "state.json")))
	app.registry = testRegistry(stubSourceFactory{source: source}, stubOutputFactory{output: output})
	app.newDeps = func(config.Config) (RuntimeDeps, error) {
		return RuntimeDeps{
			Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
			Retry: RetryPolicy{
				InitialInterval: 25 * time.Millisecond,
				MaxInterval:     50 * time.Millisecond,
				MaxElapsedTime:  200 * time.Millisecond,
			},
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	waitForCondition(t, func() bool {
		return source.listDesiredCallCount() == 2 && source.watchCallCount() == 1
	})

	first.errs <- io.EOF
	waitForConditionWithin(t, 100*time.Millisecond, func() bool {
		return source.watchCallCount() >= 2
	})
	waitForCondition(t, func() bool {
		return source.listDesiredCallCount() >= 3
	})

	second.errs <- io.EOF
	assertConditionHolds(t, 20*time.Millisecond, func() bool { return source.watchCallCount() == 2 })
	waitForConditionWithin(t, 100*time.Millisecond, func() bool {
		return source.watchCallCount() >= 3
	})
	assertRunStillRunning(t, done)

	cancel()
	assertRunStops(t, done)
}

func TestReconcileRetriesFullPassAfterTransientListVisibleFailure(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "state.json")
	store, err := state.NewStore(statePath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}
	source := &startupSourceStub{
		provider: provider,
		desired: []contracts.DesiredRecord{{
			Hostname: "stale.local",
			Answer:   "10.0.0.10",
			Source:   contracts.SourceObjectRef{Provider: provider, ID: "ctr-1", DisplayName: "svc"},
		}},
	}
	source.onListDesired = func() {
		if source.listDesiredCallCount() != 1 {
			return
		}
		source.mu.Lock()
		source.desired = []contracts.DesiredRecord{{
			Hostname: "app.local",
			Answer:   "10.0.0.10",
			Source:   contracts.SourceObjectRef{Provider: provider, ID: "ctr-1", DisplayName: "svc"},
		}}
		source.mu.Unlock()
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	deps := RuntimeDeps{
		Logger: logger,
		Retry: RetryPolicy{
			InitialInterval: time.Millisecond,
			MaxInterval:     time.Millisecond,
			MaxElapsedTime:  50 * time.Millisecond,
		},
	}

	output := &transientOutputStub{
		provider:                 contracts.ProviderRef{Type: "adguard", Name: "primary"},
		failListVisible:          1,
		failListVisibleTemporary: true,
	}

	app := New(testRuntimeConfig(statePath))
	app.deps = deps
	app.store = store
	app.sources = []contracts.Source{source}
	app.outputs = wrapOutputs([]contracts.Output{output}, deps)

	if err := app.reconcile(context.Background(), "startup"); err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}

	if source.listDesiredCallCount() != 2 {
		t.Fatalf("expected 2 ListDesired calls, got %d", source.listDesiredCallCount())
	}
	if output.listVisibleCount() != 2 {
		t.Fatalf("expected 2 ListVisible calls, got %d", output.listVisibleCount())
	}
	if output.createCount() != 1 {
		t.Fatalf("expected 1 Create call, got %d", output.createCount())
	}
	visible := output.visibleSnapshot()
	if len(visible) != 1 || visible[0].Hostname != "app.local" {
		t.Fatalf("expected reconcile retry to use refreshed desired state, got %+v", visible)
	}

	snapshot, err := store.Load()
	if err != nil {
		t.Fatalf("load persisted snapshot: %v", err)
	}
	if len(snapshot.ManagedRecords) != 1 {
		t.Fatalf("expected one persisted managed record, got %d", len(snapshot.ManagedRecords))
	}

	logs := buf.String()
	for _, want := range []string{"reason=startup", "retrying full reconcile after temporary output read failure", "operation=create", "attempt=1", "persisted state snapshot", "output mutation applied"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("expected log output to contain %q, got %s", want, logs)
		}
	}
	if strings.Contains(logs, "operation=create provider=adguard/primary hostname=stale.local") {
		t.Fatalf("expected stale desired state to be discarded before retry, got %s", logs)
	}
	if strings.Contains(logs, "secret") {
		t.Fatalf("log output leaked secret-like value: %s", logs)
	}
}

func TestReconcileRetriesFullPassAfterTransientListDesiredFailure(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "state.json")
	store, err := state.NewStore(statePath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}
	source := &startupSourceStub{
		provider:             provider,
		failListDesiredAfter: 0,
		failListDesired:      1,
		desired: []contracts.DesiredRecord{{
			Hostname: "app.local",
			Answer:   "10.0.0.10",
			Source:   contracts.SourceObjectRef{Provider: provider, ID: "ctr-1", DisplayName: "svc"},
		}},
	}
	output := &startupOutputStub{provider: contracts.ProviderRef{Type: "adguard", Name: "primary"}}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	deps := RuntimeDeps{
		Logger: logger,
		Retry: RetryPolicy{
			InitialInterval: time.Millisecond,
			MaxInterval:     time.Millisecond,
			MaxElapsedTime:  50 * time.Millisecond,
		},
	}

	app := New(testRuntimeConfig(statePath))
	app.deps = deps
	app.store = store
	app.sources = []contracts.Source{source}
	app.outputs = wrapOutputs([]contracts.Output{output}, deps)

	if err := app.reconcile(context.Background(), "startup"); err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}

	if source.listDesiredCallCount() != 2 {
		t.Fatalf("expected 2 ListDesired calls, got %d", source.listDesiredCallCount())
	}
	if output.listVisibleCount() != 1 {
		t.Fatalf("expected 1 ListVisible call after desired-state retry, got %d", output.listVisibleCount())
	}
	if output.createCount() != 1 {
		t.Fatalf("expected 1 Create call, got %d", output.createCount())
	}

	snapshot, err := store.Load()
	if err != nil {
		t.Fatalf("load persisted snapshot: %v", err)
	}
	if len(snapshot.ManagedRecords) != 1 {
		t.Fatalf("expected one persisted managed record, got %d", len(snapshot.ManagedRecords))
	}

	logs := buf.String()
	for _, want := range []string{"retrying full reconcile after temporary source read failure", "reason=startup", "source=docker/local", "attempt=1"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("expected log output to contain %q, got %s", want, logs)
		}
	}
	if strings.Contains(logs, "secret") {
		t.Fatalf("log output leaked secret-like value: %s", logs)
	}
}

func TestReconcileRetriesFullPassAfterTransientCreateFailure(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "state.json")
	store, err := state.NewStore(statePath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	deps := RuntimeDeps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
		Retry: RetryPolicy{
			InitialInterval: time.Millisecond,
			MaxInterval:     time.Millisecond,
			MaxElapsedTime:  3 * time.Millisecond,
		},
	}

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}
	source := &startupSourceStub{
		provider: provider,
		desired: []contracts.DesiredRecord{{
			Hostname: "app.local",
			Answer:   "10.0.0.10",
			Source:   contracts.SourceObjectRef{Provider: provider, ID: "ctr-1", DisplayName: "svc"},
		}},
	}
	output := &transientOutputStub{
		provider:            contracts.ProviderRef{Type: "adguard", Name: "primary"},
		failCreate:          1,
		failCreateTemporary: true,
		failWithMessage:     "adguard unavailable",
	}
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	app := New(testRuntimeConfig(statePath))
	app.deps = RuntimeDeps{
		Logger: logger,
		Retry:  deps.Retry,
	}
	app.store = store
	app.sources = []contracts.Source{source}
	app.outputs = wrapOutputs([]contracts.Output{output}, app.deps)

	err = app.reconcile(context.Background(), "startup")
	if err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}
	if output.createCount() != 2 {
		t.Fatalf("expected 2 Create calls, got %d", output.createCount())
	}
	if output.listVisibleCount() != 2 {
		t.Fatalf("expected 2 ListVisible calls, got %d", output.listVisibleCount())
	}
	if source.listDesiredCallCount() != 2 {
		t.Fatalf("expected 2 ListDesired calls, got %d", source.listDesiredCallCount())
	}

	snapshot, loadErr := store.Load()
	if loadErr != nil {
		t.Fatalf("load snapshot: %v", loadErr)
	}
	if len(snapshot.ManagedRecords) != 1 {
		t.Fatalf("expected one persisted managed record, got %d", len(snapshot.ManagedRecords))
	}

	logs := buf.String()
	for _, want := range []string{"retrying full reconcile after temporary output write failure", "reason=startup", "attempt=1", "operation=create"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("expected log output to contain %q, got %s", want, logs)
		}
	}
	if strings.Contains(logs, "secret") {
		t.Fatalf("log output leaked secret-like value: %s", logs)
	}
	if visible := output.visibleSnapshot(); len(visible) != 1 || visible[0].Hostname != "app.local" {
		t.Fatalf("unexpected visible records after retry: %+v", visible)
	}
	if strings.Count(logs, "retrying full reconcile after temporary output write failure") != 1 {
		t.Fatalf("expected exactly one full-reconcile write retry, got logs %s", logs)
	}
	if source.listDesiredCallCount() != output.listVisibleCount() {
		t.Fatalf("expected full reconcile retry to re-read desired and visible state together, got desired=%d visible=%d", source.listDesiredCallCount(), output.listVisibleCount())
	}
	if output.createCount() != 2 {
		t.Fatalf("expected transient write retry to remain bounded, got %d create calls", output.createCount())
	}
	if output.listVisibleCount() != 2 {
		t.Fatalf("expected bounded full reconcile attempts, got %d visible reads", output.listVisibleCount())
	}
	if output.createCount() > output.listVisibleCount() {
		t.Fatalf("expected full reconcile retry instead of inline mutation retries, got create=%d visible=%d", output.createCount(), output.listVisibleCount())
	}
	if output.createCount() > 2 {
		t.Fatalf("expected bounded retry behavior, got %d create calls", output.createCount())
	}
	if output.listVisibleCount() > 2 {
		t.Fatalf("expected bounded retry behavior, got %d visible reads", output.listVisibleCount())
	}
	if output.createCount() == 2 && !strings.Contains(logs, "reconcile completed") {
		t.Fatalf("expected reconcile completion log after transient write retry, got %s", logs)
	}
	if output.createCount() == 2 && !strings.Contains(logs, "persisted state snapshot") {
		t.Fatalf("expected persisted state log after transient write retry, got %s", logs)
	}
	if output.createCount() == 2 && !strings.Contains(logs, "adguard unavailable") {
		t.Fatalf("expected transient failure context in logs, got %s", logs)
	}
	if output.createCount() == 2 && strings.Contains(logs, "retrying full reconcile after temporary output read failure") {
		t.Fatalf("expected write retry path, got logs %s", logs)
	}
	if source.listDesiredCallCount() != 2 {
		t.Fatalf("expected exactly two full reconcile attempts, got %d desired reads", source.listDesiredCallCount())
	}
	if output.createCount() == 2 && len(output.visibleSnapshot()) != 1 {
		t.Fatalf("expected one converged visible record, got %+v", output.visibleSnapshot())
	}
	if output.createCount() == 2 && snapshot.ManagedRecords[0].Hostname != "app.local" {
		t.Fatalf("unexpected persisted record after retry: %+v", snapshot.ManagedRecords[0])
	}
}

func TestReconcileReturnsTerminalMutationErrorWithoutRetryOrPersistingState(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "state.json")
	store, err := state.NewStore(statePath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	deps := RuntimeDeps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
		Retry: RetryPolicy{
			InitialInterval: time.Millisecond,
			MaxInterval:     time.Millisecond,
			MaxElapsedTime:  3 * time.Millisecond,
		},
	}

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}
	source := &startupSourceStub{
		provider: provider,
		desired: []contracts.DesiredRecord{{
			Hostname: "app.local",
			Answer:   "10.0.0.10",
			Source:   contracts.SourceObjectRef{Provider: provider, ID: "ctr-1", DisplayName: "svc"},
		}},
	}
	output := &transientOutputStub{
		provider:        contracts.ProviderRef{Type: "adguard", Name: "primary"},
		failCreate:      1,
		failWithMessage: "adguard unavailable",
	}
	app := New(testRuntimeConfig(statePath))
	app.deps = deps
	app.store = store
	app.sources = []contracts.Source{source}
	app.outputs = wrapOutputs([]contracts.Output{output}, deps)

	err = app.reconcile(context.Background(), "startup")
	if err == nil {
		t.Fatal("expected reconcile error")
	}
	if output.createCount() != 1 {
		t.Fatalf("expected 1 Create call, got %d", output.createCount())
	}
	if output.listVisibleCount() != 1 {
		t.Fatalf("expected 1 ListVisible call, got %d", output.listVisibleCount())
	}
	if source.listDesiredCallCount() != 1 {
		t.Fatalf("expected 1 ListDesired call, got %d", source.listDesiredCallCount())
	}

	snapshot, loadErr := store.Load()
	if loadErr != nil {
		t.Fatalf("load snapshot: %v", loadErr)
	}
	if len(snapshot.ManagedRecords) != 0 {
		t.Fatalf("expected empty snapshot after mutation failure, got %d records", len(snapshot.ManagedRecords))
	}
}

type startupSourceStub struct {
	provider             contracts.ProviderRef
	desired              []contracts.DesiredRecord
	listDesiredCalls     int
	onListDesired        func()
	failListDesiredAfter int
	failListDesired      int
	failWithMessage      string
	mu                   sync.Mutex
}

func (s *startupSourceStub) Provider() contracts.ProviderRef {
	return s.provider
}

func (s *startupSourceStub) ListDesired(context.Context) ([]contracts.DesiredRecord, error) {
	s.mu.Lock()
	s.listDesiredCalls++
	call := s.listDesiredCalls
	s.mu.Unlock()
	if s.onListDesired != nil {
		s.onListDesired()
	}
	if call > s.failListDesiredAfter && call <= s.failListDesiredAfter+s.failListDesired {
		msg := s.failWithMessage
		if msg == "" {
			msg = "transient source failure"
		}
		return nil, stubFailure(msg, true)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	desired := append([]contracts.DesiredRecord(nil), s.desired...)
	return desired, nil
}

func (s *startupSourceStub) listDesiredCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listDesiredCalls
}

type startupOutputStub struct {
	provider         contracts.ProviderRef
	visible          []contracts.VisibleRecord
	created          []contracts.DesiredRecord
	listVisibleCalls int
	mu               sync.Mutex
}

func (o *startupOutputStub) Provider() contracts.ProviderRef {
	return o.provider
}

func (o *startupOutputStub) ListVisible(context.Context) ([]contracts.VisibleRecord, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.listVisibleCalls++
	return append([]contracts.VisibleRecord(nil), o.visible...), nil
}

func (o *startupOutputStub) Create(_ context.Context, desired contracts.DesiredRecord) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.created = append(o.created, desired)
	o.visible = append(o.visible, contracts.VisibleRecord{Output: o.provider, Hostname: desired.Hostname, Answer: desired.Answer})
	return nil
}

func (o *startupOutputStub) Update(context.Context, contracts.VisibleRecord, contracts.DesiredRecord) error {
	return nil
}

func (o *startupOutputStub) Delete(context.Context, contracts.VisibleRecord) error {
	return nil
}

func (o *startupOutputStub) createCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.created)
}

func (o *startupOutputStub) listVisibleCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.listVisibleCalls
}

type watchSession struct {
	hints chan struct{}
	errs  chan error
}

func newWatchSession() *watchSession {
	return &watchSession{
		hints: make(chan struct{}, 1),
		errs:  make(chan error, 1),
	}
}

type watchSourceStub struct {
	startupSourceStub
	sessions   []*watchSession
	watchCalls int
	watchMu    sync.Mutex
}

func (s *watchSourceStub) Watch(context.Context) contracts.SourceWatch {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	idx := s.watchCalls
	s.watchCalls++
	if idx >= len(s.sessions) {
		closedHints := make(chan struct{})
		closedErrs := make(chan error)
		close(closedHints)
		close(closedErrs)
		return contracts.SourceWatch{Hints: closedHints, Err: closedErrs}
	}

	session := s.sessions[idx]
	return contracts.SourceWatch{Hints: session.hints, Err: session.errs}
}

func (s *watchSourceStub) watchCallCount() int {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	return s.watchCalls
}

type transientOutputStub struct {
	provider                 contracts.ProviderRef
	failListVisible          int
	failListVisibleTemporary bool
	failCreateAfter          int
	failCreate               int
	failCreateTemporary      bool
	failWithMessage          string
	listVisibleCalls         int
	createCalls              int
	visible                  []contracts.VisibleRecord
	onCreateFailureHook      func()
	mu                       sync.Mutex
}

func (o *transientOutputStub) Provider() contracts.ProviderRef {
	return o.provider
}

func (o *transientOutputStub) ListVisible(context.Context) ([]contracts.VisibleRecord, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.listVisibleCalls++
	if o.listVisibleCalls <= o.failListVisible {
		return nil, stubFailure("transient list failure", o.failListVisibleTemporary)
	}
	return append([]contracts.VisibleRecord(nil), o.visible...), nil
}

func (o *transientOutputStub) Create(_ context.Context, desired contracts.DesiredRecord) error {
	o.mu.Lock()
	o.createCalls++
	shouldFail := o.createCalls > o.failCreateAfter && o.createCalls <= o.failCreateAfter+o.failCreate
	hook := o.onCreateFailureHook
	msg := o.failWithMessage
	o.mu.Unlock()
	if shouldFail {
		if hook != nil {
			hook()
		}
		if msg == "" {
			msg = "transient create failure"
		}
		return stubFailure(msg, o.failCreateTemporary)
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	o.visible = append(o.visible, contracts.VisibleRecord{Output: o.provider, Hostname: desired.Hostname, Answer: desired.Answer})
	return nil
}

func (o *transientOutputStub) Update(context.Context, contracts.VisibleRecord, contracts.DesiredRecord) error {
	return nil
}

func (o *transientOutputStub) Delete(context.Context, contracts.VisibleRecord) error {
	return nil
}

func (o *transientOutputStub) createCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.createCalls
}

func (o *transientOutputStub) listVisibleCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.listVisibleCalls
}

func (o *transientOutputStub) visibleSnapshot() []contracts.VisibleRecord {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]contracts.VisibleRecord(nil), o.visible...)
}

type stubTemporaryError struct {
	err error
}

func (e stubTemporaryError) Error() string {
	return e.err.Error()
}

func (e stubTemporaryError) Unwrap() error {
	return e.err
}

func (e stubTemporaryError) Temporary() bool {
	return true
}

func stubFailure(message string, temporary bool) error {
	err := errors.New(message)
	if temporary {
		return stubTemporaryError{err: err}
	}
	return err
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
	waitForConditionWithin(t, 2*time.Second, condition)
}

func waitForConditionWithin(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("condition was not met before timeout")
}

func assertConditionHolds(t *testing.T, duration time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		if !condition() {
			t.Fatal("condition changed before duration elapsed")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func assertRunStops(t *testing.T, done <-chan error) {
	t.Helper()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for shutdown")
	}
}

func assertRunStillRunning(t *testing.T, done <-chan error) {
	t.Helper()

	select {
	case err := <-done:
		t.Fatalf("run exited unexpectedly: %v", err)
	default:
	}
}
