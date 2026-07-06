package runtime

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/aaronflorey/docker-dns-sync/internal/contracts"
	"github.com/aaronflorey/docker-dns-sync/internal/state"
)

func TestPersistedManagedRecords(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "adguard", Name: "primary"}
	source := contracts.SourceObjectRef{Provider: contracts.ProviderRef{Type: "docker", Name: "local"}, ID: "ctr-1", DisplayName: "svc"}

	t.Run("persists only after successful apply", func(t *testing.T) {
		t.Parallel()

		statePath := filepath.Join(t.TempDir(), "state.json")
		store, err := state.NewStore(statePath)
		if err != nil {
			t.Fatalf("new store: %v", err)
		}

		fakeOutput := &reconcileFakeOutputFailCreate{
			provider: provider,
			err:      errors.New("create failed"),
		}

		before, err := store.Load()
		if err != nil {
			t.Fatalf("load before: %v", err)
		}

		_, err = ReconcileAndPersist(context.Background(), store, ReconcileInput{
			Output: fakeOutput,
			Desired: []contracts.DesiredRecord{{
				Hostname: "app.local",
				Answer:   "10.0.0.10",
				Source:   source,
			}},
			Visible: nil,
			Owned:   before,
		})
		if err == nil {
			t.Fatalf("expected apply error")
		}

		after, err := store.Load()
		if err != nil {
			t.Fatalf("load after: %v", err)
		}
		if len(after.ManagedRecords) != len(before.ManagedRecords) {
			t.Fatalf("expected unchanged snapshot on apply failure")
		}
	})

	t.Run("records traceability and last applied from success path", func(t *testing.T) {
		t.Parallel()

		statePath := filepath.Join(t.TempDir(), "state.json")
		store, err := state.NewStore(statePath)
		if err != nil {
			t.Fatalf("new store: %v", err)
		}

		now := time.Date(2026, 5, 13, 2, 0, 0, 0, time.UTC)
		fakeOutput := &reconcileFakeOutput{provider: provider}

		current, err := store.Load()
		if err != nil {
			t.Fatalf("load current: %v", err)
		}

		result, err := ReconcileAndPersist(context.Background(), store, ReconcileInput{
			Output: fakeOutput,
			Desired: []contracts.DesiredRecord{{
				Hostname: "app.local",
				Answer:   "10.0.0.10",
				Source:   source,
			}},
			Visible: nil,
			Owned:   current,
			Now: func() time.Time {
				return now
			},
		})
		if err != nil {
			t.Fatalf("reconcile and persist: %v", err)
		}

		if len(result.Next.ManagedRecords) != 1 {
			t.Fatalf("expected one managed record, got %d", len(result.Next.ManagedRecords))
		}

		record := result.Next.ManagedRecords[0]
		if record.Output != provider {
			t.Fatalf("unexpected output traceability: %+v", record.Output)
		}
		if record.Source != source {
			t.Fatalf("unexpected source traceability: %+v", record.Source)
		}
		if record.Hostname != "app.local" || record.Answer != "10.0.0.10" {
			t.Fatalf("unexpected managed record fields: %+v", record)
		}
		if !record.LastAppliedAt.Equal(now) {
			t.Fatalf("expected LastAppliedAt=%s got %s", now, record.LastAppliedAt)
		}

		saved, err := store.Load()
		if err != nil {
			t.Fatalf("load saved: %v", err)
		}
		if len(saved.ManagedRecords) != 1 {
			t.Fatalf("expected one persisted record, got %d", len(saved.ManagedRecords))
		}
	})

	t.Run("persists successful mutation progress before later failure", func(t *testing.T) {
		t.Parallel()

		statePath := filepath.Join(t.TempDir(), "state.json")
		store, err := state.NewStore(statePath)
		if err != nil {
			t.Fatalf("new store: %v", err)
		}

		fakeOutput := &reconcilePartialCreateOutput{provider: provider}
		current, err := store.Load()
		if err != nil {
			t.Fatalf("load current: %v", err)
		}

		_, err = ReconcileAndPersist(context.Background(), store, ReconcileInput{
			Output: fakeOutput,
			Desired: []contracts.DesiredRecord{
				{Hostname: "app-1.local", Answer: "10.0.0.10", Source: contracts.SourceObjectRef{Provider: source.Provider, ID: "ctr-1", DisplayName: "svc-1"}},
				{Hostname: "app-2.local", Answer: "10.0.0.11", Source: contracts.SourceObjectRef{Provider: source.Provider, ID: "ctr-2", DisplayName: "svc-2"}},
			},
			Visible: nil,
			Owned:   current,
		})
		if err == nil {
			t.Fatal("expected apply error")
		}

		saved, err := store.Load()
		if err != nil {
			t.Fatalf("load saved: %v", err)
		}
		if len(saved.ManagedRecords) != 1 {
			t.Fatalf("expected one persisted managed record after partial failure, got %d", len(saved.ManagedRecords))
		}
		if saved.ManagedRecords[0].Hostname != "app-1.local" || saved.ManagedRecords[0].Answer != "10.0.0.10" {
			t.Fatalf("unexpected persisted partial progress: %+v", saved.ManagedRecords[0])
		}
	})

	t.Run("retains old lineage when replacement create fails after earlier progress", func(t *testing.T) {
		t.Parallel()

		statePath := filepath.Join(t.TempDir(), "state.json")
		store, err := state.NewStore(statePath)
		if err != nil {
			t.Fatalf("new store: %v", err)
		}

		seeded := state.Snapshot{ManagedRecords: []state.ManagedRecord{{
			Output:   provider,
			Source:   contracts.SourceObjectRef{Provider: source.Provider, ID: "ctr-old", DisplayName: "svc-old"},
			Hostname: "app-2.local",
			Answer:   "10.0.0.11",
		}}}
		if err := store.Save(seeded); err != nil {
			t.Fatalf("seed store: %v", err)
		}

		current, err := store.Load()
		if err != nil {
			t.Fatalf("load current: %v", err)
		}

		fakeOutput := &reconcilePartialCreateOutput{provider: provider}
		_, err = ReconcileAndPersist(context.Background(), store, ReconcileInput{
			Output: fakeOutput,
			Desired: []contracts.DesiredRecord{
				{Hostname: "app-1.local", Answer: "10.0.0.10", Source: contracts.SourceObjectRef{Provider: source.Provider, ID: "ctr-1", DisplayName: "svc-1"}},
				{Hostname: "app-2.local", Answer: "10.0.0.11", Source: contracts.SourceObjectRef{Provider: source.Provider, ID: "ctr-2", DisplayName: "svc-2"}},
			},
			Visible: nil,
			Owned:   current,
		})
		if err == nil {
			t.Fatal("expected apply error")
		}

		saved, err := store.Load()
		if err != nil {
			t.Fatalf("load saved: %v", err)
		}
		if len(saved.ManagedRecords) != 2 {
			t.Fatalf("expected successful create plus retained old lineage after partial failure, got %d records", len(saved.ManagedRecords))
		}
		if saved.ManagedRecords[0].Hostname != "app-1.local" || saved.ManagedRecords[0].Source.ID != "ctr-1" {
			t.Fatalf("expected first successful create to persist, got %+v", saved.ManagedRecords[0])
		}
		if saved.ManagedRecords[1].Hostname != "app-2.local" || saved.ManagedRecords[1].Source.ID != "ctr-old" || saved.ManagedRecords[1].Answer != "10.0.0.11" {
			t.Fatalf("expected failed replacement to retain old lineage, got %+v", saved.ManagedRecords[1])
		}
	})
}

type reconcileFakeOutputFailCreate struct {
	provider contracts.ProviderRef
	err      error
}

func (f *reconcileFakeOutputFailCreate) Provider() contracts.ProviderRef { return f.provider }

func (f *reconcileFakeOutputFailCreate) ListVisible(context.Context) ([]contracts.VisibleRecord, error) {
	return nil, nil
}

func (f *reconcileFakeOutputFailCreate) Create(context.Context, contracts.DesiredRecord) (*contracts.RecordProvenance, error) {
	return nil, f.err
}

func (f *reconcileFakeOutputFailCreate) Update(context.Context, contracts.VisibleRecord, contracts.DesiredRecord) (*contracts.RecordProvenance, error) {
	return nil, nil
}

func (f *reconcileFakeOutputFailCreate) Delete(context.Context, contracts.VisibleRecord) error {
	return nil
}

type reconcilePartialCreateOutput struct {
	provider    contracts.ProviderRef
	createCalls int
}

func (f *reconcilePartialCreateOutput) Provider() contracts.ProviderRef { return f.provider }

func (f *reconcilePartialCreateOutput) ListVisible(context.Context) ([]contracts.VisibleRecord, error) {
	return nil, nil
}

func (f *reconcilePartialCreateOutput) Create(context.Context, contracts.DesiredRecord) (*contracts.RecordProvenance, error) {
	f.createCalls++
	if f.createCalls == 2 {
		return nil, errors.New("create failed")
	}
	return nil, nil
}

func (f *reconcilePartialCreateOutput) Update(context.Context, contracts.VisibleRecord, contracts.DesiredRecord) (*contracts.RecordProvenance, error) {
	return nil, nil
}

func (f *reconcilePartialCreateOutput) Delete(context.Context, contracts.VisibleRecord) error {
	return nil
}
