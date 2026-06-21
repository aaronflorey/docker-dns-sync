package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aaronflorey/docker-dns-sync/internal/contracts"
	"github.com/aaronflorey/docker-dns-sync/internal/state"
)

func TestReconcilePlanApply(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "adguard", Name: "primary"}
	source := contracts.SourceObjectRef{Provider: contracts.ProviderRef{Type: "docker", Name: "local"}, ID: "ctr-1", DisplayName: "svc"}

	t.Run("create missing desired records", func(t *testing.T) {
		t.Parallel()

		fakeOutput := &reconcileFakeOutput{provider: provider}
		now := time.Date(2026, 5, 13, 1, 0, 0, 0, time.UTC)

		result, err := ReconcileOutput(context.Background(), ReconcileInput{
			Output: fakeOutput,
			Desired: []contracts.DesiredRecord{{Hostname: "app.local", Answer: "10.0.0.10", Source: source}},
			Visible: nil,
			Owned:   state.EmptySnapshot(),
			Now: func() time.Time {
				return now
			},
		})
		if err != nil {
			t.Fatalf("reconcile output: %v", err)
		}

		if got := len(fakeOutput.created); got != 1 {
			t.Fatalf("expected 1 create call, got %d", got)
		}
		if len(result.Next.ManagedRecords) != 1 {
			t.Fatalf("expected 1 managed record, got %d", len(result.Next.ManagedRecords))
		}
		if !result.Next.ManagedRecords[0].LastAppliedAt.Equal(now) {
			t.Fatalf("expected LastAppliedAt %s, got %s", now, result.Next.ManagedRecords[0].LastAppliedAt)
		}
	})

	t.Run("update owned record on answer drift", func(t *testing.T) {
		t.Parallel()

		fakeOutput := &reconcileFakeOutput{provider: provider}
		owned := state.Snapshot{ManagedRecords: []state.ManagedRecord{{
			Output:   provider,
			Source:   source,
			Hostname: "app.local",
			Answer:   "10.0.0.10",
		}}}

		_, err := ReconcileOutput(context.Background(), ReconcileInput{
			Output: fakeOutput,
			Desired: []contracts.DesiredRecord{{Hostname: "app.local", Answer: "10.0.0.11", Source: source}},
			Visible: []contracts.VisibleRecord{{Output: provider, Hostname: "APP.LOCAL", Answer: "10.0.0.10"}},
			Owned:   owned,
		})
		if err != nil {
			t.Fatalf("reconcile output: %v", err)
		}

		if got := len(fakeOutput.updated); got != 1 {
			t.Fatalf("expected 1 update call, got %d", got)
		}
		if got := len(fakeOutput.deleted); got != 0 {
			t.Fatalf("expected no delete calls, got %d", got)
		}
	})

	t.Run("update owned record when singleton visible hostname drifted away from old answer", func(t *testing.T) {
		t.Parallel()

		fakeOutput := &reconcileFakeOutput{provider: provider}
		nextSource := contracts.SourceObjectRef{Provider: source.Provider, ID: "ctr-2", DisplayName: source.DisplayName}
		owned := state.Snapshot{ManagedRecords: []state.ManagedRecord{{
			Output:   provider,
			Source:   source,
			Hostname: "s3.local",
			Answer:   "origin.internal",
		}}}

		result, err := ReconcileOutput(context.Background(), ReconcileInput{
			Output: fakeOutput,
			Desired: []contracts.DesiredRecord{{Hostname: "s3.local", Answer: "10.0.0.11", Source: nextSource}},
			Visible: []contracts.VisibleRecord{{Output: provider, Hostname: "S3.local", Answer: "192.0.2.5"}},
			Owned:   owned,
		})
		if err != nil {
			t.Fatalf("reconcile output: %v", err)
		}

		if got := len(fakeOutput.created); got != 0 {
			t.Fatalf("expected no create calls, got %d", got)
		}
		if got := len(fakeOutput.updated); got != 1 {
			t.Fatalf("expected 1 update call, got %d", got)
		}
		if from := fakeOutput.updated[0].From; from.Hostname != "S3.local" || from.Answer != "192.0.2.5" {
			t.Fatalf("unexpected update source: %+v", from)
		}
		if got := len(result.Next.ManagedRecords); got != 1 || result.Next.ManagedRecords[0].Answer != "10.0.0.11" {
			t.Fatalf("unexpected next managed records: %+v", result.Next.ManagedRecords)
		}
		if result.Next.ManagedRecords[0].Source.ID != "ctr-2" {
			t.Fatalf("expected managed source ID to roll forward, got %+v", result.Next.ManagedRecords[0].Source)
		}
	})

	t.Run("preserve owned record when source ID changes but desired record stays the same", func(t *testing.T) {
		t.Parallel()

		fakeOutput := &reconcileFakeOutput{provider: provider}
		nextSource := contracts.SourceObjectRef{Provider: source.Provider, ID: "ctr-2", DisplayName: source.DisplayName}
		owned := state.Snapshot{ManagedRecords: []state.ManagedRecord{{
			Output:   provider,
			Source:   source,
			Hostname: "app.local",
			Answer:   "10.0.0.10",
		}}}

		result, err := ReconcileOutput(context.Background(), ReconcileInput{
			Output:  fakeOutput,
			Desired: []contracts.DesiredRecord{{Hostname: "app.local", Answer: "10.0.0.10", Source: nextSource}},
			Visible: []contracts.VisibleRecord{{Output: provider, Hostname: "app.local", Answer: "10.0.0.10"}},
			Owned:   owned,
		})
		if err != nil {
			t.Fatalf("reconcile output: %v", err)
		}

		if got := len(fakeOutput.created); got != 0 {
			t.Fatalf("expected no create calls, got %d", got)
		}
		if got := len(fakeOutput.updated); got != 0 {
			t.Fatalf("expected no update calls, got %d", got)
		}
		if got := len(fakeOutput.deleted); got != 0 {
			t.Fatalf("expected no delete calls, got %d", got)
		}
		if got := len(result.Next.ManagedRecords); got != 1 {
			t.Fatalf("expected 1 managed record, got %d", got)
		}
		if result.Next.ManagedRecords[0].Source.ID != "ctr-2" {
			t.Fatalf("expected managed source ID to roll forward, got %+v", result.Next.ManagedRecords[0].Source)
		}
	})

	t.Run("update owned record when source identity changes but hostname stays unique", func(t *testing.T) {
		t.Parallel()

		fakeOutput := &reconcileFakeOutput{provider: provider}
		nextSource := contracts.SourceObjectRef{Provider: source.Provider, ID: "ctr-9", DisplayName: "svc-recreated"}
		owned := state.Snapshot{ManagedRecords: []state.ManagedRecord{{
			Output:   provider,
			Source:   source,
			Hostname: "s3.local",
			Answer:   "origin.internal",
		}}}

		result, err := ReconcileOutput(context.Background(), ReconcileInput{
			Output: fakeOutput,
			Desired: []contracts.DesiredRecord{{Hostname: "s3.local", Answer: "192.168.1.142", Source: nextSource}},
			Visible: []contracts.VisibleRecord{{Output: provider, Hostname: "s3.local", Answer: "legacy.internal"}},
			Owned:   owned,
		})
		if err != nil {
			t.Fatalf("reconcile output: %v", err)
		}

		if got := len(fakeOutput.created); got != 0 {
			t.Fatalf("expected no create calls, got %d", got)
		}
		if got := len(fakeOutput.updated); got != 1 {
			t.Fatalf("expected 1 update call, got %d", got)
		}
		if got := len(result.Next.ManagedRecords); got != 1 {
			t.Fatalf("expected 1 managed record, got %d", got)
		}
		if result.Next.ManagedRecords[0].Source.DisplayName != "svc-recreated" || result.Next.ManagedRecords[0].Source.ID != "ctr-9" {
			t.Fatalf("expected managed source identity to roll forward, got %+v", result.Next.ManagedRecords[0].Source)
		}
	})

	t.Run("drop stale owned records without deleting remote dns", func(t *testing.T) {
		t.Parallel()

		fakeOutput := &reconcileFakeOutput{provider: provider}
		owned := state.Snapshot{ManagedRecords: []state.ManagedRecord{{
			Output:   provider,
			Source:   source,
			Hostname: "old.local",
			Answer:   "10.0.0.12",
		}}}

		result, err := ReconcileOutput(context.Background(), ReconcileInput{
			Output:  fakeOutput,
			Desired: nil,
			Visible: []contracts.VisibleRecord{{Output: provider, Hostname: "old.local", Answer: "10.0.0.12"}},
			Owned:   owned,
		})
		if err != nil {
			t.Fatalf("reconcile output: %v", err)
		}

		if got := len(fakeOutput.deleted); got != 0 {
			t.Fatalf("expected no delete calls, got %d", got)
		}
		if got := len(result.Next.ManagedRecords); got != 0 {
			t.Fatalf("expected stale owned state to be dropped, got %d managed records", got)
		}
	})

	t.Run("recreate owned records missing from visible state during restart recovery", func(t *testing.T) {
		t.Parallel()

		fakeOutput := &reconcileFakeOutput{provider: provider}
		now := time.Date(2026, 5, 14, 1, 0, 0, 0, time.UTC)
		owned := state.Snapshot{ManagedRecords: []state.ManagedRecord{{
			Output:   provider,
			Source:   source,
			Hostname: "app.local",
			Answer:   "10.0.0.10",
		}}}

		result, err := ReconcileOutput(context.Background(), ReconcileInput{
			Output: fakeOutput,
			Desired: []contracts.DesiredRecord{{
				Hostname: "app.local",
				Answer:   "10.0.0.10",
				Source:   source,
			}},
			Visible: nil,
			Owned:   owned,
			Now: func() time.Time {
				return now
			},
		})
		if err != nil {
			t.Fatalf("reconcile output: %v", err)
		}

		if got := len(fakeOutput.created); got != 1 {
			t.Fatalf("expected 1 recovery create call, got %d", got)
		}
		if got := len(result.Next.ManagedRecords); got != 1 {
			t.Fatalf("expected 1 managed record after recovery create, got %d", got)
		}
		if !result.Next.ManagedRecords[0].LastAppliedAt.Equal(now) {
			t.Fatalf("expected LastAppliedAt %s, got %s", now, result.Next.ManagedRecords[0].LastAppliedAt)
		}
	})

	t.Run("drop stale owned state when record is already gone", func(t *testing.T) {
		t.Parallel()

		fakeOutput := &reconcileFakeOutput{provider: provider}
		owned := state.Snapshot{ManagedRecords: []state.ManagedRecord{{
			Output:   provider,
			Source:   source,
			Hostname: "gone.local",
			Answer:   "10.0.0.99",
		}}}

		result, err := ReconcileOutput(context.Background(), ReconcileInput{
			Output:  fakeOutput,
			Desired: nil,
			Visible: nil,
			Owned:   owned,
		})
		if err != nil {
			t.Fatalf("reconcile output: %v", err)
		}

		if got := len(fakeOutput.deleted); got != 0 {
			t.Fatalf("expected no delete calls when visible rewrite is already gone, got %d", got)
		}
		if got := len(result.Next.ManagedRecords); got != 0 {
			t.Fatalf("expected stale owned state to be dropped, got %d managed records", got)
		}
	})
}

func TestOwnershipBoundary(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "adguard", Name: "primary"}
	fakeOutput := &reconcileFakeOutput{provider: provider}

	_, err := ReconcileOutput(context.Background(), ReconcileInput{
		Output:  fakeOutput,
		Desired: nil,
		Visible: []contracts.VisibleRecord{{Output: provider, Hostname: "manual.local", Answer: "10.0.0.50"}},
		Owned:   state.EmptySnapshot(),
	})
	if err != nil {
		t.Fatalf("reconcile output: %v", err)
	}

	if got := len(fakeOutput.deleted); got != 0 {
		t.Fatalf("expected no delete calls for unmanaged records, got %d", got)
	}
	if got := len(fakeOutput.updated); got != 0 {
		t.Fatalf("expected no update calls for unmanaged records, got %d", got)
	}
}

func TestPreserveManualRecords(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "adguard", Name: "primary"}
	source := contracts.SourceObjectRef{Provider: contracts.ProviderRef{Type: "docker", Name: "local"}, ID: "ctr-1", DisplayName: "svc"}
	fakeOutput := &reconcileFakeOutput{provider: provider}

	_, err := ReconcileOutput(context.Background(), ReconcileInput{
		Output: fakeOutput,
		Desired: []contracts.DesiredRecord{{Hostname: "app.local", Answer: "10.0.0.10", Source: source}},
		Visible: []contracts.VisibleRecord{
			{Output: provider, Hostname: "app.local", Answer: "10.0.0.10"},
			{Output: provider, Hostname: "APP.local", Answer: "10.0.0.10"},
		},
		Owned: state.EmptySnapshot(),
	})
	if err == nil {
		t.Fatalf("expected ambiguity error for duplicate visible keys")
	}
	var ambiguityErr *ErrVisibleRecordAmbiguous
	if !errors.As(err, &ambiguityErr) {
		t.Fatalf("expected ErrVisibleRecordAmbiguous, got %T", err)
	}
	if got := len(fakeOutput.deleted); got != 0 {
		t.Fatalf("expected non-destructive duplicate handling, got %d deletes", got)
	}
}

func TestFilterDesiredForOutput(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "adguard", Name: "primary"}
	source := contracts.SourceObjectRef{Provider: contracts.ProviderRef{Type: "docker", Name: "local"}, ID: "ctr-1", DisplayName: "svc"}
	desired := []contracts.DesiredRecord{
		{Hostname: "all.local", Answer: "10.0.0.10", Source: source},
		{Hostname: "adguard.local", Answer: "10.0.0.11", Source: source, Output: "adguard"},
		{Hostname: "cloudflare.local", Answer: "10.0.0.12", Source: source, Output: "cloudflare"},
	}

	filtered := filterDesiredForOutput(desired, provider)
	if got := len(filtered); got != 2 {
		t.Fatalf("expected 2 filtered records, got %d (%+v)", got, filtered)
	}
	if filtered[0].Hostname != "all.local" || filtered[1].Hostname != "adguard.local" {
		t.Fatalf("unexpected filtered records: %+v", filtered)
	}
}

type reconcileFakeOutput struct {
	provider contracts.ProviderRef
	created  []contracts.DesiredRecord
	updated  []reconcileUpdateCall
	deleted  []contracts.VisibleRecord
}

func (f *reconcileFakeOutput) Provider() contracts.ProviderRef { return f.provider }

func (f *reconcileFakeOutput) ListVisible(context.Context) ([]contracts.VisibleRecord, error) {
	return nil, nil
}

func (f *reconcileFakeOutput) Create(_ context.Context, desired contracts.DesiredRecord) error {
	f.created = append(f.created, desired)
	return nil
}

func (f *reconcileFakeOutput) Update(_ context.Context, visible contracts.VisibleRecord, desired contracts.DesiredRecord) error {
	f.updated = append(f.updated, reconcileUpdateCall{From: visible, To: desired})
	return nil
}

func (f *reconcileFakeOutput) Delete(_ context.Context, visible contracts.VisibleRecord) error {
	f.deleted = append(f.deleted, visible)
	return nil
}
