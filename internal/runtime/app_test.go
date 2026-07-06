package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

const shortWatchHintDebounce = 20 * time.Millisecond

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

func TestAppReconcileOnceFiltersDesiredRecordsPerOutput(t *testing.T) {
	t.Parallel()

	adguardProvider := contracts.ProviderRef{Type: "adguard", Name: "primary"}
	cloudflareProvider := contracts.ProviderRef{Type: "cloudflare", Name: "primary"}
	sourceRef := contracts.SourceObjectRef{Provider: contracts.ProviderRef{Type: "docker", Name: "local"}, ID: "ctr-1", DisplayName: "svc"}
	adguardOutput := &startupOutputStub{provider: adguardProvider}
	cloudflareOutput := &startupOutputStub{provider: cloudflareProvider}
	statePath := filepath.Join(t.TempDir(), "state.json")

	store, err := state.NewStore(statePath)
	if err != nil {
		t.Fatalf("create state store: %v", err)
	}

	app := New(testRuntimeConfig(statePath))
	app.store = store
	setTestProviders(app, testRuntimeDeps(RetryPolicy{}, 0), []contracts.Source{&startupSourceStub{provider: sourceRef.Provider, desired: []contracts.DesiredRecord{
		{Hostname: "shared.local", Answer: "10.0.0.10", Source: sourceRef},
		{Hostname: "adguard.local", Answer: "10.0.0.11", Source: sourceRef, Output: "adguard"},
		{Hostname: "cloudflare.local", Answer: "10.0.0.12", Source: sourceRef, Output: "cloudflare"},
	}}}, []contracts.Output{adguardOutput, cloudflareOutput})

	if err := app.reconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile once: %v", err)
	}

	if got := adguardOutput.createCount(); got != 2 {
		t.Fatalf("expected 2 adguard create calls, got %d", got)
	}
	if got := cloudflareOutput.createCount(); got != 2 {
		t.Fatalf("expected 2 cloudflare create calls, got %d", got)
	}
	for _, record := range adguardOutput.created {
		if record.Output == "cloudflare" {
			t.Fatalf("adguard output received cloudflare-only record: %+v", adguardOutput.created)
		}
	}
	for _, record := range cloudflareOutput.created {
		if record.Output == "adguard" {
			t.Fatalf("cloudflare output received adguard-only record: %+v", cloudflareOutput.created)
		}
	}
}

func TestAppReconcileOnceAppliesOperationTimeoutToWrappedSourcesAndOutputs(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "state.json")
	store, err := state.NewStore(statePath)
	if err != nil {
		t.Fatalf("create state store: %v", err)
	}

	timeout := 50 * time.Millisecond
	deps := testRuntimeDeps(RetryPolicy{}, 0)
	deps.OperationTimeout = timeout

	source := &deadlineObservingSourceStub{provider: contracts.ProviderRef{Type: "docker", Name: "local"}}
	output := &deadlineObservingOutputStub{provider: contracts.ProviderRef{Type: "adguard", Name: "primary"}}

	app := New(testRuntimeConfig(statePath))
	app.store = store
	setTestProviders(app, deps, []contracts.Source{source}, []contracts.Output{output})

	startedAt := time.Now()
	if err := app.reconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile once: %v", err)
	}
	finishedAt := time.Now()

	assertObservedDeadline(t, source.observedDeadline(), startedAt, finishedAt, timeout)
	assertObservedDeadline(t, output.observedDeadline(), startedAt, finishedAt, timeout)
}

func TestReconcileReturnsPromptlyAfterSourceListTimeout(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "state.json")
	store, err := state.NewStore(statePath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	timeout := 100 * time.Millisecond
	assertionWindow := 500 * time.Millisecond
	deps := testRuntimeDeps(RetryPolicy{
		InitialInterval: 350 * time.Millisecond,
		MaxInterval:     350 * time.Millisecond,
		MaxElapsedTime:  2 * time.Second,
	}, 0)
	deps.OperationTimeout = timeout

	source := &blockingSourceStub{provider: contracts.ProviderRef{Type: "docker", Name: "local"}}
	output := &startupOutputStub{provider: contracts.ProviderRef{Type: "adguard", Name: "primary"}}

	app := New(testRuntimeConfig(statePath))
	app.store = store
	setTestProviders(app, deps, []contracts.Source{source}, []contracts.Output{output})

	startedAt, finishedAt, err := runReconcileWithin(t, app, timeout, assertionWindow)
	assertTimeoutError(t, err)
	assertObservedDeadline(t, source.observedDeadline(), startedAt, finishedAt, timeout)
	if got := source.listDesiredCallCount(); got != 1 {
		t.Fatalf("expected 1 ListDesired call without retry, got %d", got)
	}
	if got := output.listVisibleCount(); got != 0 {
		t.Fatalf("expected no ListVisible calls after source timeout, got %d", got)
	}
}

func TestReconcileReturnsPromptlyAfterOutputListTimeout(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "state.json")
	store, err := state.NewStore(statePath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	timeout := 100 * time.Millisecond
	assertionWindow := 500 * time.Millisecond
	deps := testRuntimeDeps(RetryPolicy{
		InitialInterval: 350 * time.Millisecond,
		MaxInterval:     350 * time.Millisecond,
		MaxElapsedTime:  2 * time.Second,
	}, 0)
	deps.OperationTimeout = timeout

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}
	source := &startupSourceStub{provider: provider, desired: []contracts.DesiredRecord{{
		Hostname: "app.local",
		Answer:   "10.0.0.10",
		Source:   contracts.SourceObjectRef{Provider: provider, ID: "ctr-1", DisplayName: "svc"},
	}}}
	output := &blockingOutputStub{provider: contracts.ProviderRef{Type: "adguard", Name: "primary"}, blockOperation: blockingOutputListVisible}

	app := New(testRuntimeConfig(statePath))
	app.store = store
	setTestProviders(app, deps, []contracts.Source{source}, []contracts.Output{output})

	startedAt, finishedAt, err := runReconcileWithin(t, app, timeout, assertionWindow)
	assertTimeoutError(t, err)
	assertObservedDeadline(t, output.observedDeadline(blockingOutputListVisible), startedAt, finishedAt, timeout)
	if got := source.listDesiredCallCount(); got != 1 {
		t.Fatalf("expected 1 ListDesired call without retry, got %d", got)
	}
	if got := output.listVisibleCount(); got != 1 {
		t.Fatalf("expected 1 ListVisible call without retry, got %d", got)
	}
	if got := output.createCount(); got != 0 {
		t.Fatalf("expected no Create calls after output list timeout, got %d", got)
	}
}

func TestReconcileReturnsPromptlyAfterOutputMutationTimeout(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}
	outputProvider := contracts.ProviderRef{Type: "adguard", Name: "primary"}
	sourceRef := contracts.SourceObjectRef{Provider: provider, ID: "ctr-1", DisplayName: "svc"}

	tests := []struct {
		name       string
		operation  blockingOutputOperation
		seedOwned  []state.ManagedRecord
		desired    []contracts.DesiredRecord
		visible    []contracts.VisibleRecord
		wantCreate int
		wantUpdate int
		wantDelete int
	}{
		{
			name:      "create",
			operation: blockingOutputCreate,
			desired: []contracts.DesiredRecord{{
				Hostname: "create.local",
				Answer:   "10.0.0.10",
				Source:   sourceRef,
			}},
			wantCreate: 1,
		},
		{
			name:      "update",
			operation: blockingOutputUpdate,
			seedOwned: []state.ManagedRecord{{
				Output:     outputProvider,
				Source:     sourceRef,
				Hostname:   "update.local",
				Answer:     "10.0.0.20",
				Provenance: &contracts.RecordProvenance{RemoteID: "update-rec"},
			}},
			desired: []contracts.DesiredRecord{{
				Hostname: "update.local",
				Answer:   "10.0.0.21",
				Source:   sourceRef,
			}},
			visible: []contracts.VisibleRecord{{
				Output:     outputProvider,
				Hostname:   "update.local",
				Answer:     "10.0.0.20",
				Provenance: &contracts.RecordProvenance{RemoteID: "update-rec"},
			}},
			wantUpdate: 1,
		},
		{
			name:      "delete",
			operation: blockingOutputDelete,
			seedOwned: []state.ManagedRecord{{
				Output:     outputProvider,
				Source:     sourceRef,
				Hostname:   "delete.local",
				Answer:     "10.0.0.30",
				Provenance: &contracts.RecordProvenance{RemoteID: "delete-rec"},
			}},
			visible: []contracts.VisibleRecord{{
				Output:     outputProvider,
				Hostname:   "delete.local",
				Answer:     "10.0.0.30",
				Provenance: &contracts.RecordProvenance{RemoteID: "delete-rec"},
			}},
			wantDelete: 1,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			statePath := filepath.Join(t.TempDir(), "state.json")
			store, err := state.NewStore(statePath)
			if err != nil {
				t.Fatalf("create store: %v", err)
			}
			if len(tc.seedOwned) > 0 {
				if err := store.Save(state.Snapshot{ManagedRecords: append([]state.ManagedRecord(nil), tc.seedOwned...)}); err != nil {
					t.Fatalf("seed state store: %v", err)
				}
			}

			timeout := 100 * time.Millisecond
			assertionWindow := 500 * time.Millisecond
			deps := testRuntimeDeps(RetryPolicy{
				InitialInterval: 350 * time.Millisecond,
				MaxInterval:     350 * time.Millisecond,
				MaxElapsedTime:  2 * time.Second,
			}, 0)
			deps.OperationTimeout = timeout

			source := &startupSourceStub{provider: provider, desired: append([]contracts.DesiredRecord(nil), tc.desired...)}
			output := &blockingOutputStub{
				provider:       outputProvider,
				visible:        append([]contracts.VisibleRecord(nil), tc.visible...),
				blockOperation: tc.operation,
			}

			app := New(testRuntimeConfig(statePath))
			app.store = store
			setTestProviders(app, deps, []contracts.Source{source}, []contracts.Output{output})

			startedAt, finishedAt, err := runReconcileWithin(t, app, timeout, assertionWindow)
			assertTimeoutError(t, err)
			assertObservedDeadline(t, output.observedDeadline(tc.operation), startedAt, finishedAt, timeout)
			if got := source.listDesiredCallCount(); got != 1 {
				t.Fatalf("expected 1 ListDesired call without retry, got %d", got)
			}
			if got := output.listVisibleCount(); got != 1 {
				t.Fatalf("expected 1 ListVisible call without retry, got %d", got)
			}
			if got := output.createCount(); got != tc.wantCreate {
				t.Fatalf("expected %d Create calls, got %d", tc.wantCreate, got)
			}
			if got := output.updateCount(); got != tc.wantUpdate {
				t.Fatalf("expected %d Update calls, got %d", tc.wantUpdate, got)
			}
			if got := output.deleteCount(); got != tc.wantDelete {
				t.Fatalf("expected %d Delete calls, got %d", tc.wantDelete, got)
			}
		})
	}
}

func TestReconcileDoesNotRetryTemporaryContextCancellationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		err             error
		source          contracts.Source
		output          contracts.Output
		wantSourceCalls int
		wantListVisible int
		wantCreateCalls int
		wantDeleteCalls int
	}{
		{
			name:            "source deadline exceeded",
			err:             stubTemporaryError{err: context.DeadlineExceeded},
			source:          &errorSourceStub{provider: contracts.ProviderRef{Type: "docker", Name: "local"}, err: stubTemporaryError{err: context.DeadlineExceeded}},
			output:          &startupOutputStub{provider: contracts.ProviderRef{Type: "adguard", Name: "primary"}},
			wantSourceCalls: 1,
		},
		{
			name: "output canceled",
			err:  stubTemporaryError{err: context.Canceled},
			source: &startupSourceStub{provider: contracts.ProviderRef{Type: "docker", Name: "local"}, desired: []contracts.DesiredRecord{{
				Hostname: "app.local",
				Answer:   "10.0.0.10",
				Source:   contracts.SourceObjectRef{Provider: contracts.ProviderRef{Type: "docker", Name: "local"}, ID: "ctr-1", DisplayName: "svc"},
			}}},
			output:          &errorOutputStub{provider: contracts.ProviderRef{Type: "adguard", Name: "primary"}, listErr: stubTemporaryError{err: context.Canceled}},
			wantSourceCalls: 1,
			wantListVisible: 1,
		},
		{
			name: "output mutation deadline exceeded",
			err:  stubTemporaryError{err: context.DeadlineExceeded},
			source: &startupSourceStub{provider: contracts.ProviderRef{Type: "docker", Name: "local"}, desired: []contracts.DesiredRecord{{
				Hostname: "app.local",
				Answer:   "10.0.0.10",
				Source:   contracts.SourceObjectRef{Provider: contracts.ProviderRef{Type: "docker", Name: "local"}, ID: "ctr-1", DisplayName: "svc"},
			}}},
			output:          &errorOutputStub{provider: contracts.ProviderRef{Type: "adguard", Name: "primary"}, createErr: stubTemporaryError{err: context.DeadlineExceeded}},
			wantSourceCalls: 1,
			wantListVisible: 1,
			wantCreateCalls: 1,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			statePath := filepath.Join(t.TempDir(), "state.json")
			store, err := state.NewStore(statePath)
			if err != nil {
				t.Fatalf("create store: %v", err)
			}

			deps := testRuntimeDeps(RetryPolicy{
				InitialInterval: 350 * time.Millisecond,
				MaxInterval:     350 * time.Millisecond,
				MaxElapsedTime:  2 * time.Second,
			}, 0)

			app := New(testRuntimeConfig(statePath))
			app.store = store
			setTestProviders(app, deps, []contracts.Source{tc.source}, []contracts.Output{tc.output})

			_, _, err = runReconcileWithin(t, app, 0, 100*time.Millisecond)
			if !errors.Is(err, tc.err) {
				t.Fatalf("expected reconcile to return %v, got %v", tc.err, err)
			}

			switch source := tc.source.(type) {
			case *errorSourceStub:
				if got := source.listDesiredCallCount(); got != tc.wantSourceCalls {
					t.Fatalf("expected %d ListDesired calls, got %d", tc.wantSourceCalls, got)
				}
			case *startupSourceStub:
				if got := source.listDesiredCallCount(); got != tc.wantSourceCalls {
					t.Fatalf("expected %d ListDesired calls, got %d", tc.wantSourceCalls, got)
				}
			}

			if output, ok := tc.output.(*errorOutputStub); ok {
				if got := output.listVisibleCount(); got != tc.wantListVisible {
					t.Fatalf("expected %d ListVisible calls, got %d", tc.wantListVisible, got)
				}
				if got := output.createCount(); got != tc.wantCreateCalls {
					t.Fatalf("expected %d Create calls, got %d", tc.wantCreateCalls, got)
				}
				if got := output.deleteCount(); got != tc.wantDeleteCalls {
					t.Fatalf("expected %d Delete calls, got %d", tc.wantDeleteCalls, got)
				}
			}
		})
	}
}

func TestAppRunStartupUpdatesOwnedHostnameWhenVisibleRecordDrifted(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "adguard", Name: "primary"}
	oldSourceRef := contracts.SourceObjectRef{Provider: contracts.ProviderRef{Type: "docker", Name: "local"}, ID: "ctr-1", DisplayName: "svc"}
	sourceRef := contracts.SourceObjectRef{Provider: oldSourceRef.Provider, ID: "ctr-2", DisplayName: oldSourceRef.DisplayName}
	statePath := filepath.Join(t.TempDir(), "state.json")

	store, err := state.NewStore(statePath)
	if err != nil {
		t.Fatalf("create state store: %v", err)
	}
	if err := store.Save(state.Snapshot{ManagedRecords: []state.ManagedRecord{{
		Output:     provider,
		Source:     oldSourceRef,
		Hostname:   "s3.local",
		Answer:     "origin.internal",
		Provenance: &contracts.RecordProvenance{RemoteID: "rec-existing"},
	}}}); err != nil {
		t.Fatalf("seed state store: %v", err)
	}

	output := &startupOutputStub{
		provider: provider,
		visible:  []contracts.VisibleRecord{{Output: provider, Hostname: "s3.local", Answer: "legacy.internal", Provenance: &contracts.RecordProvenance{RemoteID: "rec-existing"}}},
	}
	app := New(testRuntimeConfig(statePath))
	app.registry = testRegistry(
		stubSourceFactory{source: &startupSourceStub{provider: sourceRef.Provider, desired: []contracts.DesiredRecord{{Hostname: "s3.local", Answer: "192.168.1.142", Source: sourceRef}}}},
		stubOutputFactory{output: output},
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx)
	}()

	waitForCondition(t, func() bool {
		return output.updateCount() == 1
	})
	if got := output.createCount(); got != 0 {
		t.Fatalf("expected no create calls, got %d", got)
	}

	snapshot, err := store.Load()
	if err != nil {
		t.Fatalf("load persisted snapshot: %v", err)
	}
	if len(snapshot.ManagedRecords) != 1 {
		t.Fatalf("expected one persisted managed record, got %d", len(snapshot.ManagedRecords))
	}
	if snapshot.ManagedRecords[0].Answer != "192.168.1.142" {
		t.Fatalf("unexpected persisted record: %+v", snapshot.ManagedRecords[0])
	}

	cancel()
	assertRunStops(t, done)
}

func TestAppRunStartupUpdatesOwnedHostnameWhenContainerIdentityChanges(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "adguard", Name: "primary"}
	oldSourceRef := contracts.SourceObjectRef{Provider: contracts.ProviderRef{Type: "docker", Name: "local"}, ID: "ctr-1", DisplayName: "svc"}
	sourceRef := contracts.SourceObjectRef{Provider: oldSourceRef.Provider, ID: "ctr-9", DisplayName: "svc-recreated"}
	statePath := filepath.Join(t.TempDir(), "state.json")

	store, err := state.NewStore(statePath)
	if err != nil {
		t.Fatalf("create state store: %v", err)
	}
	if err := store.Save(state.Snapshot{ManagedRecords: []state.ManagedRecord{{
		Output:     provider,
		Source:     oldSourceRef,
		Hostname:   "s3.local",
		Answer:     "origin.internal",
		Provenance: &contracts.RecordProvenance{RemoteID: "rec-existing"},
	}}}); err != nil {
		t.Fatalf("seed state store: %v", err)
	}

	output := &startupOutputStub{
		provider: provider,
		visible:  []contracts.VisibleRecord{{Output: provider, Hostname: "s3.local", Answer: "legacy.internal", Provenance: &contracts.RecordProvenance{RemoteID: "rec-existing"}}},
	}
	app := New(testRuntimeConfig(statePath))
	app.registry = testRegistry(
		stubSourceFactory{source: &startupSourceStub{provider: sourceRef.Provider, desired: []contracts.DesiredRecord{{Hostname: "s3.local", Answer: "192.168.1.142", Source: sourceRef}}}},
		stubOutputFactory{output: output},
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx)
	}()

	waitForCondition(t, func() bool {
		return output.updateCount() == 1
	})
	if got := output.createCount(); got != 0 {
		t.Fatalf("expected no create calls, got %d", got)
	}

	snapshot, err := store.Load()
	if err != nil {
		t.Fatalf("load persisted snapshot: %v", err)
	}
	if len(snapshot.ManagedRecords) != 1 {
		t.Fatalf("expected one persisted managed record, got %d", len(snapshot.ManagedRecords))
	}
	if snapshot.ManagedRecords[0].Source.ID != "ctr-9" || snapshot.ManagedRecords[0].Source.DisplayName != "svc-recreated" {
		t.Fatalf("unexpected persisted source identity: %+v", snapshot.ManagedRecords[0].Source)
	}

	cancel()
	assertRunStops(t, done)
}

func TestAppRunStartupDeletesSeededStaleOwnedRecordWithMatchingProvenanceAndPersistsEmptyState(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "adguard", Name: "primary"}
	sourceRef := contracts.SourceObjectRef{Provider: contracts.ProviderRef{Type: "docker", Name: "local"}, ID: "ctr-1", DisplayName: "svc"}
	statePath := filepath.Join(t.TempDir(), "state.json")

	store, err := state.NewStore(statePath)
	if err != nil {
		t.Fatalf("create state store: %v", err)
	}
	seed := state.Snapshot{ManagedRecords: []state.ManagedRecord{{
		Output:     provider,
		Source:     sourceRef,
		Hostname:   "old.local",
		Answer:     "10.0.0.12",
		Provenance: &contracts.RecordProvenance{RemoteID: "rec-stale"},
	}}}
	if err := store.Save(seed); err != nil {
		t.Fatalf("seed state store: %v", err)
	}

	deleteObserved := make(chan state.Snapshot, 1)
	deleteErrs := make(chan error, 1)
	output := &startupOutputStub{
		provider: provider,
		visible: []contracts.VisibleRecord{{
			Output:     provider,
			Hostname:   "OLD.local",
			Answer:     "10.0.0.12",
			Provenance: &contracts.RecordProvenance{RemoteID: "rec-stale"},
		}},
		onDelete: func() {
			snapshot, err := store.Load()
			if err != nil {
				deleteErrs <- err
				return
			}
			deleteObserved <- snapshot
		},
	}
	app := New(testRuntimeConfig(statePath))
	app.registry = testRegistry(
		stubSourceFactory{source: &startupSourceStub{provider: sourceRef.Provider}},
		stubOutputFactory{output: output},
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx)
	}()

	waitForCondition(t, func() bool {
		return output.deleteCount() == 1
	})

	select {
	case err := <-deleteErrs:
		t.Fatalf("load state during delete: %v", err)
	case snapshot := <-deleteObserved:
		if len(snapshot.ManagedRecords) != 1 {
			t.Fatalf("expected seeded state to remain persisted during delete, got %+v", snapshot.ManagedRecords)
		}
		got := snapshot.ManagedRecords[0]
		if got.Output != seed.ManagedRecords[0].Output || got.Source != seed.ManagedRecords[0].Source || got.Hostname != seed.ManagedRecords[0].Hostname || got.Answer != seed.ManagedRecords[0].Answer || got.Provenance == nil || got.Provenance.RemoteID != "rec-stale" {
			t.Fatalf("expected seeded state to remain persisted during delete, got %+v", snapshot.ManagedRecords)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out observing delete ordering")
	}

	snapshot, err := store.Load()
	if err != nil {
		t.Fatalf("load persisted snapshot: %v", err)
	}
	if got := output.deleteCount(); got != 1 {
		t.Fatalf("expected one delete call, got %d", got)
	}
	if got := len(snapshot.ManagedRecords); got != 0 {
		t.Fatalf("expected empty persisted managed snapshot, got %d records", got)
	}
	if visible := output.visibleSnapshot(); len(visible) != 0 {
		t.Fatalf("expected delete to remove visible record, got %+v", visible)
	}

	cancel()
	assertRunStops(t, done)
}

func TestAppRunStartupRetainsSameKeyVisibleRecordStateWithoutProvenanceProof(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "adguard", Name: "primary"}
	sourceRef := contracts.SourceObjectRef{Provider: contracts.ProviderRef{Type: "docker", Name: "local"}, ID: "ctr-1", DisplayName: "svc"}
	statePath := filepath.Join(t.TempDir(), "state.json")

	store, err := state.NewStore(statePath)
	if err != nil {
		t.Fatalf("create state store: %v", err)
	}
	if err := store.Save(state.Snapshot{ManagedRecords: []state.ManagedRecord{{
		Output:   provider,
		Source:   sourceRef,
		Hostname: "old.local",
		Answer:   "10.0.0.12",
	}}}); err != nil {
		t.Fatalf("seed state store: %v", err)
	}

	output := &startupOutputStub{
		provider: provider,
		visible: []contracts.VisibleRecord{{
			Output:   provider,
			Hostname: "OLD.local",
			Answer:   "10.0.0.12",
		}},
	}
	app := New(testRuntimeConfig(statePath))
	app.registry = testRegistry(
		stubSourceFactory{source: &startupSourceStub{provider: sourceRef.Provider}},
		stubOutputFactory{output: output},
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx)
	}()

	waitForCondition(t, func() bool {
		snapshot, err := store.Load()
		return err == nil && len(snapshot.ManagedRecords) == 1
	})

	if got := output.deleteCount(); got != 0 {
		t.Fatalf("expected no delete calls without provenance proof, got %d", got)
	}
	snapshot, err := store.Load()
	if err != nil {
		t.Fatalf("load persisted snapshot: %v", err)
	}
	if got := len(snapshot.ManagedRecords); got != 1 {
		t.Fatalf("expected stale owned state to be retained, got %d managed records", got)
	}
	if snapshot.ManagedRecords[0].Hostname != "old.local" || snapshot.ManagedRecords[0].Answer != "10.0.0.12" {
		t.Fatalf("expected retained stale owned record to stay unchanged, got %+v", snapshot.ManagedRecords[0])
	}
	if visible := output.visibleSnapshot(); len(visible) != 1 {
		t.Fatalf("expected visible record to remain untouched, got %+v", visible)
	}

	cancel()
	assertRunStops(t, done)
}

func TestAppReconcileOnceRetainsStaleOwnedStateWithoutDeletingSameKeyVisibleRecord(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "adguard", Name: "primary"}
	sourceRef := contracts.SourceObjectRef{Provider: contracts.ProviderRef{Type: "docker", Name: "local"}, ID: "ctr-1", DisplayName: "svc"}
	statePath := filepath.Join(t.TempDir(), "state.json")

	store, err := state.NewStore(statePath)
	if err != nil {
		t.Fatalf("create state store: %v", err)
	}
	if err := store.Save(state.Snapshot{ManagedRecords: []state.ManagedRecord{{
		Output:   provider,
		Source:   sourceRef,
		Hostname: "old.local",
		Answer:   "10.0.0.12",
	}}}); err != nil {
		t.Fatalf("seed state store: %v", err)
	}

	output := &startupOutputStub{
		provider: provider,
		visible: []contracts.VisibleRecord{
			{Output: provider, Hostname: "OLD.local", Answer: "10.0.0.12"},
			{Output: provider, Hostname: "manual.local", Answer: "10.0.0.50"},
		},
	}
	app := New(testRuntimeConfig(statePath))
	app.store = store
	setTestProviders(app, testRuntimeDeps(RetryPolicy{}, 0), []contracts.Source{&startupSourceStub{provider: sourceRef.Provider}}, []contracts.Output{output})

	if err := app.reconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile once: %v", err)
	}
	if got := output.deleteCount(); got != 0 {
		t.Fatalf("expected no delete calls without unique ownership proof, got %d", got)
	}

	snapshot, err := store.Load()
	if err != nil {
		t.Fatalf("load persisted snapshot: %v", err)
	}
	if got := len(snapshot.ManagedRecords); got != 1 {
		t.Fatalf("expected stale owned state to be retained, got %d managed records", got)
	}
	if snapshot.ManagedRecords[0].Hostname != "old.local" || snapshot.ManagedRecords[0].Answer != "10.0.0.12" {
		t.Fatalf("expected retained stale owned record to stay unchanged, got %+v", snapshot.ManagedRecords[0])
	}
	if visible := output.visibleSnapshot(); len(visible) != 2 {
		t.Fatalf("expected visible records to remain untouched, got %+v", visible)
	}
}

func TestAppReconcileOnceDoesNotDeleteOrPersistAcrossAmbiguousVisibleDuplicates(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "adguard", Name: "primary"}
	sourceRef := contracts.SourceObjectRef{Provider: contracts.ProviderRef{Type: "docker", Name: "local"}, ID: "ctr-1", DisplayName: "svc"}
	statePath := filepath.Join(t.TempDir(), "state.json")

	store, err := state.NewStore(statePath)
	if err != nil {
		t.Fatalf("create state store: %v", err)
	}
	seed := state.Snapshot{ManagedRecords: []state.ManagedRecord{{
		Output:   provider,
		Source:   sourceRef,
		Hostname: "old.local",
		Answer:   "10.0.0.12",
	}}}
	if err := store.Save(seed); err != nil {
		t.Fatalf("seed state store: %v", err)
	}

	output := &startupOutputStub{
		provider: provider,
		visible: []contracts.VisibleRecord{
			{Output: provider, Hostname: "old.local", Answer: "10.0.0.12"},
			{Output: provider, Hostname: "OLD.local", Answer: "10.0.0.12"},
			{Output: provider, Hostname: "manual.local", Answer: "10.0.0.50"},
		},
	}
	app := New(testRuntimeConfig(statePath))
	app.store = store
	setTestProviders(app, testRuntimeDeps(RetryPolicy{}, 0), []contracts.Source{&startupSourceStub{provider: sourceRef.Provider}}, []contracts.Output{output})

	err = app.reconcileOnce(context.Background())
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
	var ambiguityErr *ErrVisibleRecordAmbiguous
	if !errors.As(err, &ambiguityErr) {
		t.Fatalf("expected ErrVisibleRecordAmbiguous, got %T", err)
	}
	if got := output.deleteCount(); got != 0 {
		t.Fatalf("expected duplicate visible handling to remain non-destructive, got %d deletes", got)
	}

	snapshot, loadErr := store.Load()
	if loadErr != nil {
		t.Fatalf("load persisted snapshot: %v", loadErr)
	}
	if len(snapshot.ManagedRecords) != len(seed.ManagedRecords) || snapshot.ManagedRecords[0] != seed.ManagedRecords[0] {
		t.Fatalf("expected persisted state to remain unchanged on ambiguity, got %+v", snapshot.ManagedRecords)
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
	session := &watchSession{hints: make(chan struct{}, 3), errs: make(chan error, 1)}
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
		return testRuntimeDeps(RetryPolicy{
			InitialInterval: 10 * time.Millisecond,
			MaxInterval:     20 * time.Millisecond,
			MaxElapsedTime:  200 * time.Millisecond,
		}, shortWatchHintDebounce), nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	waitForCondition(t, func() bool {
		return source.listDesiredCallCount() == 2 && output.createCount() == 1 && output.listVisibleCount() == 2
	})

	session.hints <- struct{}{}
	waitForConditionWithin(t, 250*time.Millisecond, func() bool {
		return source.listDesiredCallCount() >= 3 && output.listVisibleCount() >= 3
	})

	cancel()
	assertRunStops(t, done)
}

func TestAppRunCoalescesWatchHintsWithinDebounceWindow(t *testing.T) {
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
		return testRuntimeDeps(RetryPolicy{
			InitialInterval: 10 * time.Millisecond,
			MaxInterval:     20 * time.Millisecond,
			MaxElapsedTime:  200 * time.Millisecond,
		}, shortWatchHintDebounce), nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	waitForCondition(t, func() bool {
		return source.listDesiredCallCount() == 2 && output.listVisibleCount() == 2 && source.watchCallCount() == 1
	})

	session.hints <- struct{}{}
	session.hints <- struct{}{}
	session.hints <- struct{}{}

	waitForConditionWithin(t, 250*time.Millisecond, func() bool {
		return source.listDesiredCallCount() == 3 && output.listVisibleCount() == 3
	})
	assertConditionHolds(t, 50*time.Millisecond, func() bool {
		return source.listDesiredCallCount() == 3 && output.listVisibleCount() == 3
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
		return testRuntimeDeps(RetryPolicy{
			InitialInterval: 10 * time.Millisecond,
			MaxInterval:     20 * time.Millisecond,
			MaxElapsedTime:  200 * time.Millisecond,
		}, shortWatchHintDebounce), nil
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
	waitForConditionWithin(t, 250*time.Millisecond, func() bool {
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
		return testRuntimeDeps(RetryPolicy{
			InitialInterval: time.Millisecond,
			MaxInterval:     2 * time.Millisecond,
			MaxElapsedTime:  50 * time.Millisecond,
		}, shortWatchHintDebounce), nil
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
	waitForConditionWithin(t, 250*time.Millisecond, func() bool {
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
		return testRuntimeDeps(RetryPolicy{
			InitialInterval: time.Millisecond,
			MaxInterval:     2 * time.Millisecond,
			MaxElapsedTime:  50 * time.Millisecond,
		}, shortWatchHintDebounce), nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	waitForCondition(t, func() bool {
		return source.listDesiredCallCount() == 2 && output.createCount() == 1
	})

	session.hints <- struct{}{}
	waitForConditionWithin(t, 250*time.Millisecond, func() bool {
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
			Logger:           slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
			OperationTimeout: defaultOperationTimeout,
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
		Logger:           slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
		OperationTimeout: defaultOperationTimeout,
		Retry: RetryPolicy{
			InitialInterval: time.Millisecond,
			MaxInterval:     2 * time.Millisecond,
			MaxElapsedTime:  50 * time.Millisecond,
		},
		WatchHintDebounce: shortWatchHintDebounce,
	}

	app := New(testRuntimeConfig(statePath))
	app.deps = deps
	app.store = store
	setTestProviders(app, deps, []contracts.Source{watchSource, otherSource}, []contracts.Output{output})

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
	waitForConditionWithin(t, 250*time.Millisecond, func() bool {
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
		return testRuntimeDeps(RetryPolicy{
			InitialInterval: time.Millisecond,
			MaxInterval:     2 * time.Millisecond,
			MaxElapsedTime:  50 * time.Millisecond,
		}, shortWatchHintDebounce), nil
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
			Logger:           slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
			OperationTimeout: defaultOperationTimeout,
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
			Logger:           slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
			OperationTimeout: defaultOperationTimeout,
			Retry: RetryPolicy{
				InitialInterval: 10 * time.Millisecond,
				MaxInterval:     20 * time.Millisecond,
				MaxElapsedTime:  200 * time.Millisecond,
			},
		}, nil
	}
	app.newDeps = func(config.Config) (RuntimeDeps, error) {
		return RuntimeDeps{
			Logger:           slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
			OperationTimeout: defaultOperationTimeout,
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
		Logger:           slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
		OperationTimeout: defaultOperationTimeout,
		Retry: RetryPolicy{
			InitialInterval: 100 * time.Millisecond,
			MaxInterval:     100 * time.Millisecond,
			MaxElapsedTime:  300 * time.Millisecond,
		},
		WatchHintDebounce: shortWatchHintDebounce,
	}

	app := New(testRuntimeConfig(filepath.Join(t.TempDir(), "state.json")))
	app.deps = deps
	store, err := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	app.store = store
	setTestProviders(app, deps, []contracts.Source{firstSource, secondSource}, []contracts.Output{output})

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

	waitForConditionWithin(t, 80*time.Millisecond, func() bool {
		return firstSource.listDesiredCallCount() >= 3 && secondSource.listDesiredCallCount() >= 3 && output.listVisibleCount() >= 3
	})
	if firstSource.watchCallCount() != 1 {
		t.Fatalf("expected reconnect backoff to defer first source restart, got %d watch calls", firstSource.watchCallCount())
	}

	waitForConditionWithin(t, 220*time.Millisecond, func() bool {
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
			Logger:           slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
			OperationTimeout: defaultOperationTimeout,
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
		Logger:           logger,
		OperationTimeout: defaultOperationTimeout,
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
	setTestProviders(app, deps, []contracts.Source{source}, []contracts.Output{output})

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
		Logger:           logger,
		OperationTimeout: defaultOperationTimeout,
		Retry: RetryPolicy{
			InitialInterval: time.Millisecond,
			MaxInterval:     time.Millisecond,
			MaxElapsedTime:  50 * time.Millisecond,
		},
	}

	app := New(testRuntimeConfig(statePath))
	app.deps = deps
	app.store = store
	setTestProviders(app, deps, []contracts.Source{source}, []contracts.Output{output})

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
		Logger:           slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
		OperationTimeout: defaultOperationTimeout,
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
		Logger:           logger,
		OperationTimeout: defaultOperationTimeout,
		Retry:            deps.Retry,
	}
	app.store = store
	setTestProviders(app, app.deps, []contracts.Source{source}, []contracts.Output{output})

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

func TestReconcileRetriesFullPassAfterTransientCloudflareCreateFailure(t *testing.T) {
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
			Hostname: "app.local",
			Answer:   "10.0.0.10",
			Source:   contracts.SourceObjectRef{Provider: provider, ID: "ctr-1", DisplayName: "svc"},
			Output:   "cloudflare",
		}},
	}
	output := &transientOutputStub{
		provider:            contracts.ProviderRef{Type: "cloudflare", Name: "primary"},
		failCreate:          1,
		failCreateTemporary: true,
		failWithMessage:     "cloudflare unavailable",
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	app := New(testRuntimeConfig(statePath))
	app.deps = RuntimeDeps{
		Logger:           logger,
		OperationTimeout: defaultOperationTimeout,
		Retry: RetryPolicy{
			InitialInterval: time.Millisecond,
			MaxInterval:     time.Millisecond,
			MaxElapsedTime:  3 * time.Millisecond,
		},
	}
	app.store = store
	setTestProviders(app, app.deps, []contracts.Source{source}, []contracts.Output{output})

	if err := app.reconcile(context.Background(), "startup"); err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}
	if source.listDesiredCallCount() != 2 {
		t.Fatalf("expected 2 ListDesired calls, got %d", source.listDesiredCallCount())
	}
	if output.createCount() != 2 {
		t.Fatalf("expected 2 Create calls, got %d", output.createCount())
	}
	if output.listVisibleCount() != 2 {
		t.Fatalf("expected 2 ListVisible calls, got %d", output.listVisibleCount())
	}

	snapshot, err := store.Load()
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if len(snapshot.ManagedRecords) != 1 {
		t.Fatalf("expected one persisted managed record, got %d", len(snapshot.ManagedRecords))
	}
	if got := snapshot.ManagedRecords[0]; got.Output != output.Provider() || got.Hostname != "app.local" || got.Answer != "10.0.0.10" {
		t.Fatalf("unexpected persisted record after retry: %+v", got)
	}
	if visible := output.visibleSnapshot(); len(visible) != 1 || visible[0].Output != output.Provider() || visible[0].Hostname != "app.local" {
		t.Fatalf("unexpected visible records after retry: %+v", visible)
	}

	logs := buf.String()
	for _, want := range []string{"retrying full reconcile after temporary output write failure", "output=cloudflare/primary", "reason=startup", "attempt=1", "cloudflare unavailable", "persisted state snapshot"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("expected log output to contain %q, got %s", want, logs)
		}
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
		Logger:           slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
		OperationTimeout: defaultOperationTimeout,
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
	setTestProviders(app, deps, []contracts.Source{source}, []contracts.Output{output})

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

type deadlineObservingSourceStub struct {
	provider            contracts.ProviderRef
	listDesiredDeadline time.Time
	mu                  sync.Mutex
}

func (s *deadlineObservingSourceStub) Provider() contracts.ProviderRef {
	return s.provider
}

func (s *deadlineObservingSourceStub) ListDesired(ctx context.Context) ([]contracts.DesiredRecord, error) {
	deadline, _ := ctx.Deadline()
	s.mu.Lock()
	s.listDesiredDeadline = deadline
	s.mu.Unlock()
	return nil, nil
}

func (s *deadlineObservingSourceStub) observedDeadline() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listDesiredDeadline
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
	updated          []reconcileUpdateCall
	deleted          []contracts.VisibleRecord
	onDelete         func()
	listVisibleCalls int
	mu               sync.Mutex
}

type deadlineObservingOutputStub struct {
	provider            contracts.ProviderRef
	listVisibleDeadline time.Time
	mu                  sync.Mutex
}

type blockingOutputOperation string

const (
	blockingOutputListVisible blockingOutputOperation = "list_visible"
	blockingOutputCreate      blockingOutputOperation = "create"
	blockingOutputUpdate      blockingOutputOperation = "update"
	blockingOutputDelete      blockingOutputOperation = "delete"
)

type blockingSourceStub struct {
	provider            contracts.ProviderRef
	listDesiredCalls    int
	listDesiredDeadline time.Time
	mu                  sync.Mutex
}

type blockingOutputStub struct {
	provider       contracts.ProviderRef
	visible        []contracts.VisibleRecord
	blockOperation blockingOutputOperation
	deadlines      map[blockingOutputOperation]time.Time
	listCalls      int
	createCalls    int
	updateCalls    int
	deleteCalls    int
	mu             sync.Mutex
}

type errorSourceStub struct {
	provider         contracts.ProviderRef
	err              error
	listDesiredCalls int
	mu               sync.Mutex
}

type errorOutputStub struct {
	provider    contracts.ProviderRef
	visible     []contracts.VisibleRecord
	listErr     error
	createErr   error
	updateErr   error
	deleteErr   error
	listCalls   int
	createCalls int
	updateCalls int
	deleteCalls int
	mu          sync.Mutex
}

func (s *blockingSourceStub) Provider() contracts.ProviderRef {
	return s.provider
}

func (s *blockingSourceStub) ListDesired(ctx context.Context) ([]contracts.DesiredRecord, error) {
	deadline, _ := ctx.Deadline()
	s.mu.Lock()
	s.listDesiredCalls++
	s.listDesiredDeadline = deadline
	s.mu.Unlock()
	<-ctx.Done()
	return nil, deadlineExceededFailure(ctx.Err())
}

func (s *blockingSourceStub) observedDeadline() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listDesiredDeadline
}

func (s *blockingSourceStub) listDesiredCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listDesiredCalls
}

func (s *errorSourceStub) Provider() contracts.ProviderRef {
	return s.provider
}

func (s *errorSourceStub) ListDesired(context.Context) ([]contracts.DesiredRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listDesiredCalls++
	return nil, s.err
}

func (s *errorSourceStub) listDesiredCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listDesiredCalls
}

func (o *blockingOutputStub) Provider() contracts.ProviderRef {
	return o.provider
}

func (o *blockingOutputStub) ListVisible(ctx context.Context) ([]contracts.VisibleRecord, error) {
	o.mu.Lock()
	o.listCalls++
	o.recordDeadlineLocked(blockingOutputListVisible, ctx)
	shouldBlock := o.blockOperation == blockingOutputListVisible
	visible := append([]contracts.VisibleRecord(nil), o.visible...)
	o.mu.Unlock()
	if shouldBlock {
		<-ctx.Done()
		return nil, deadlineExceededFailure(ctx.Err())
	}
	return visible, nil
}

func (o *blockingOutputStub) Create(ctx context.Context, desired contracts.DesiredRecord) (*contracts.RecordProvenance, error) {
	o.mu.Lock()
	o.createCalls++
	o.recordDeadlineLocked(blockingOutputCreate, ctx)
	shouldBlock := o.blockOperation == blockingOutputCreate
	o.mu.Unlock()
	if shouldBlock {
		<-ctx.Done()
		return nil, deadlineExceededFailure(ctx.Err())
	}
	o.mu.Lock()
	o.visible = append(o.visible, contracts.VisibleRecord{Output: o.provider, Hostname: desired.Hostname, Answer: desired.Answer})
	o.mu.Unlock()
	return nil, nil
}

func (o *blockingOutputStub) Update(ctx context.Context, visible contracts.VisibleRecord, desired contracts.DesiredRecord) (*contracts.RecordProvenance, error) {
	o.mu.Lock()
	o.updateCalls++
	o.recordDeadlineLocked(blockingOutputUpdate, ctx)
	shouldBlock := o.blockOperation == blockingOutputUpdate
	o.mu.Unlock()
	if shouldBlock {
		<-ctx.Done()
		return nil, deadlineExceededFailure(ctx.Err())
	}
	o.mu.Lock()
	for i, current := range o.visible {
		if visibleRecordKey(current.Hostname, current.Answer) != visibleRecordKey(visible.Hostname, visible.Answer) {
			continue
		}
		o.visible[i] = contracts.VisibleRecord{Output: o.provider, Hostname: desired.Hostname, Answer: desired.Answer}
		break
	}
	o.mu.Unlock()
	return nil, nil
}

func (o *blockingOutputStub) Delete(ctx context.Context, visible contracts.VisibleRecord) error {
	o.mu.Lock()
	o.deleteCalls++
	o.recordDeadlineLocked(blockingOutputDelete, ctx)
	shouldBlock := o.blockOperation == blockingOutputDelete
	o.mu.Unlock()
	if shouldBlock {
		<-ctx.Done()
		return deadlineExceededFailure(ctx.Err())
	}
	o.mu.Lock()
	for i, current := range o.visible {
		if visibleRecordKey(current.Hostname, current.Answer) != visibleRecordKey(visible.Hostname, visible.Answer) {
			continue
		}
		o.visible = append(o.visible[:i], o.visible[i+1:]...)
		break
	}
	o.mu.Unlock()
	return nil
}

func (o *blockingOutputStub) recordDeadlineLocked(operation blockingOutputOperation, ctx context.Context) {
	if o.deadlines == nil {
		o.deadlines = make(map[blockingOutputOperation]time.Time)
	}
	deadline, _ := ctx.Deadline()
	o.deadlines[operation] = deadline
}

func (o *blockingOutputStub) observedDeadline(operation blockingOutputOperation) time.Time {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.deadlines[operation]
}

func (o *blockingOutputStub) listVisibleCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.listCalls
}

func (o *blockingOutputStub) createCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.createCalls
}

func (o *blockingOutputStub) updateCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.updateCalls
}

func (o *blockingOutputStub) deleteCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.deleteCalls
}

func (o *errorOutputStub) Provider() contracts.ProviderRef {
	return o.provider
}

func (o *errorOutputStub) ListVisible(context.Context) ([]contracts.VisibleRecord, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.listCalls++
	if o.listErr != nil {
		return nil, o.listErr
	}
	return append([]contracts.VisibleRecord(nil), o.visible...), nil
}

func (o *errorOutputStub) Create(context.Context, contracts.DesiredRecord) (*contracts.RecordProvenance, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.createCalls++
	return nil, o.createErr
}

func (o *errorOutputStub) Update(context.Context, contracts.VisibleRecord, contracts.DesiredRecord) (*contracts.RecordProvenance, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.updateCalls++
	return nil, o.updateErr
}

func (o *errorOutputStub) Delete(context.Context, contracts.VisibleRecord) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.deleteCalls++
	return o.deleteErr
}

func (o *errorOutputStub) listVisibleCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.listCalls
}

func (o *errorOutputStub) createCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.createCalls
}

func (o *errorOutputStub) deleteCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.deleteCalls
}

func (o *deadlineObservingOutputStub) Provider() contracts.ProviderRef {
	return o.provider
}

func (o *deadlineObservingOutputStub) ListVisible(ctx context.Context) ([]contracts.VisibleRecord, error) {
	deadline, _ := ctx.Deadline()
	o.mu.Lock()
	o.listVisibleDeadline = deadline
	o.mu.Unlock()
	return nil, nil
}

func (o *deadlineObservingOutputStub) Create(context.Context, contracts.DesiredRecord) (*contracts.RecordProvenance, error) {
	return nil, nil
}

func (o *deadlineObservingOutputStub) Update(context.Context, contracts.VisibleRecord, contracts.DesiredRecord) (*contracts.RecordProvenance, error) {
	return nil, nil
}

func (o *deadlineObservingOutputStub) Delete(context.Context, contracts.VisibleRecord) error {
	return nil
}

func (o *deadlineObservingOutputStub) observedDeadline() time.Time {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.listVisibleDeadline
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

func (o *startupOutputStub) Create(_ context.Context, desired contracts.DesiredRecord) (*contracts.RecordProvenance, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.created = append(o.created, desired)
	o.visible = append(o.visible, contracts.VisibleRecord{Output: o.provider, Hostname: desired.Hostname, Answer: desired.Answer})
	return nil, nil
}

func (o *startupOutputStub) Update(_ context.Context, visible contracts.VisibleRecord, desired contracts.DesiredRecord) (*contracts.RecordProvenance, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.updated = append(o.updated, reconcileUpdateCall{From: visible, To: desired})
	for i, current := range o.visible {
		if visibleRecordKey(current.Hostname, current.Answer) != visibleRecordKey(visible.Hostname, visible.Answer) {
			continue
		}
		o.visible[i] = contracts.VisibleRecord{Output: o.provider, Hostname: desired.Hostname, Answer: desired.Answer}
		return nil, nil
	}
	o.visible = append(o.visible, contracts.VisibleRecord{Output: o.provider, Hostname: desired.Hostname, Answer: desired.Answer})
	return nil, nil
}

func (o *startupOutputStub) Delete(_ context.Context, visible contracts.VisibleRecord) error {
	o.mu.Lock()
	o.deleted = append(o.deleted, visible)
	for i, current := range o.visible {
		if visibleRecordKey(current.Hostname, current.Answer) != visibleRecordKey(visible.Hostname, visible.Answer) {
			continue
		}
		o.visible = append(o.visible[:i], o.visible[i+1:]...)
		break
	}
	hook := o.onDelete
	o.mu.Unlock()
	if hook != nil {
		hook()
	}
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

func (o *startupOutputStub) updateCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.updated)
}

func (o *startupOutputStub) deleteCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.deleted)
}

func (o *startupOutputStub) visibleSnapshot() []contracts.VisibleRecord {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]contracts.VisibleRecord(nil), o.visible...)
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

func (o *transientOutputStub) Create(_ context.Context, desired contracts.DesiredRecord) (*contracts.RecordProvenance, error) {
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
		return nil, stubFailure(msg, o.failCreateTemporary)
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	o.visible = append(o.visible, contracts.VisibleRecord{Output: o.provider, Hostname: desired.Hostname, Answer: desired.Answer})
	return nil, nil
}

func (o *transientOutputStub) Update(context.Context, contracts.VisibleRecord, contracts.DesiredRecord) (*contracts.RecordProvenance, error) {
	return nil, nil
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

func testRuntimeDeps(retry RetryPolicy, watchHintDebounce time.Duration) RuntimeDeps {
	return RuntimeDeps{
		Logger:            slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
		OperationTimeout:  defaultOperationTimeout,
		Retry:             retry,
		WatchHintDebounce: watchHintDebounce,
	}
}

func setTestProviders(app *App, deps RuntimeDeps, sources []contracts.Source, outputs []contracts.Output) {
	app.deps = deps
	app.setProviders(sources, outputs, deps)
}

func runReconcileWithin(t *testing.T, app *App, timeout, assertionWindow time.Duration) (time.Time, time.Time, error) {
	t.Helper()

	startedAt := time.Now()
	done := make(chan error, 1)
	go func() {
		done <- app.reconcile(context.Background(), "startup")
	}()

	select {
	case err := <-done:
		finishedAt := time.Now()
		if elapsed := finishedAt.Sub(startedAt); elapsed > assertionWindow {
			t.Fatalf("expected reconcile to return within %v for operation timeout %v, got %v", assertionWindow, timeout, elapsed)
		}
		return startedAt, finishedAt, err
	case <-time.After(assertionWindow):
		t.Fatalf("reconcile did not return within buffered assertion window %v for operation timeout %v", assertionWindow, timeout)
		return time.Time{}, time.Time{}, nil
	}
}

func assertTimeoutError(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("expected reconcile timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
}

type deadlineExceededStubError struct{}

func (deadlineExceededStubError) Error() string {
	return fmt.Sprintf("operation timed out: %v", context.DeadlineExceeded)
}

func (deadlineExceededStubError) Is(target error) bool {
	return target == context.DeadlineExceeded
}

func deadlineExceededFailure(err error) error {
	if err == nil {
		return errors.New("operation ended without deadline error")
	}
	return deadlineExceededStubError{}
}

func assertObservedDeadline(t *testing.T, deadline, startedAt, finishedAt time.Time, timeout time.Duration) {
	t.Helper()

	if deadline.IsZero() {
		t.Fatal("expected wrapped operation context to include a deadline")
	}
	if !deadline.After(startedAt) {
		t.Fatalf("expected deadline after operation start, got start=%v deadline=%v", startedAt, deadline)
	}
	if deadline.After(finishedAt.Add(timeout + 20*time.Millisecond)) {
		t.Fatalf("expected deadline near operation timeout, got finish=%v deadline=%v timeout=%v", finishedAt, deadline, timeout)
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
