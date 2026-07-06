package runtime

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
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

		fakeOutput := &reconcileFakeOutput{provider: provider, createProvenance: &contracts.RecordProvenance{RemoteID: "rec-created"}}
		now := time.Date(2026, 5, 13, 1, 0, 0, 0, time.UTC)

		result, err := ReconcileOutput(context.Background(), ReconcileInput{
			Output:  fakeOutput,
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
		if result.Next.ManagedRecords[0].Provenance == nil || result.Next.ManagedRecords[0].Provenance.RemoteID != "rec-created" {
			t.Fatalf("expected created provenance remote ID rec-created, got %+v", result.Next.ManagedRecords[0].Provenance)
		}
	})

	t.Run("update owned record on answer drift", func(t *testing.T) {
		t.Parallel()

		fakeOutput := &reconcileFakeOutput{provider: provider, updateProvenance: &contracts.RecordProvenance{RemoteID: "rec-updated"}}
		owned := state.Snapshot{ManagedRecords: []state.ManagedRecord{{
			Output:     provider,
			Source:     source,
			Hostname:   "app.local",
			Answer:     "10.0.0.10",
			Provenance: &contracts.RecordProvenance{RemoteID: "rec-old"},
		}}}

		result, err := ReconcileOutput(context.Background(), ReconcileInput{
			Output:  fakeOutput,
			Desired: []contracts.DesiredRecord{{Hostname: "app.local", Answer: "10.0.0.11", Source: source}},
			Visible: []contracts.VisibleRecord{{Output: provider, Hostname: "APP.LOCAL", Answer: "10.0.0.10", Provenance: &contracts.RecordProvenance{RemoteID: "rec-old"}}},
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
		if result.Next.ManagedRecords[0].Provenance == nil || result.Next.ManagedRecords[0].Provenance.RemoteID != "rec-updated" {
			t.Fatalf("expected updated provenance remote ID rec-updated, got %+v", result.Next.ManagedRecords[0].Provenance)
		}
	})

	t.Run("do not update exact-lineage record without owned provenance proof", func(t *testing.T) {
		t.Parallel()

		fakeOutput := &reconcileFakeOutput{provider: provider}
		owned := state.Snapshot{ManagedRecords: []state.ManagedRecord{{
			Output:   provider,
			Source:   source,
			Hostname: "app.local",
			Answer:   "10.0.0.10",
		}}}

		result, err := ReconcileOutput(context.Background(), ReconcileInput{
			Output:  fakeOutput,
			Desired: []contracts.DesiredRecord{{Hostname: "app.local", Answer: "10.0.0.11", Source: source}},
			Visible: []contracts.VisibleRecord{{Output: provider, Hostname: "app.local", Answer: "10.0.0.10", Provenance: &contracts.RecordProvenance{RemoteID: "manual-rec"}}},
			Owned:   owned,
		})
		if err != nil {
			t.Fatalf("reconcile output: %v", err)
		}

		if got := len(fakeOutput.updated); got != 0 {
			t.Fatalf("expected no update calls without exact-lineage ownership proof, got %d", got)
		}
		if got := len(fakeOutput.created); got != 0 {
			t.Fatalf("expected no create calls without exact-lineage ownership proof, got %d", got)
		}
		if got := len(fakeOutput.deleted); got != 0 {
			t.Fatalf("expected no delete calls without exact-lineage ownership proof, got %d", got)
		}
		if got := len(result.Next.ManagedRecords); got != 1 {
			t.Fatalf("expected owned state to be retained non-destructively without exact-lineage proof, got %+v", result.Next.ManagedRecords)
		}
		if result.Next.ManagedRecords[0].Provenance != nil {
			t.Fatalf("expected retained managed record to preserve nil provenance, got %+v", result.Next.ManagedRecords[0].Provenance)
		}
	})

	t.Run("update owned record when singleton visible hostname drifted away from old answer", func(t *testing.T) {
		t.Parallel()

		fakeOutput := &reconcileFakeOutput{provider: provider}
		nextSource := contracts.SourceObjectRef{Provider: source.Provider, ID: "ctr-2", DisplayName: source.DisplayName}
		owned := state.Snapshot{ManagedRecords: []state.ManagedRecord{{
			Output:     provider,
			Source:     source,
			Hostname:   "s3.local",
			Answer:     "origin.internal",
			Provenance: &contracts.RecordProvenance{RemoteID: "rec-existing"},
		}}}

		result, err := ReconcileOutput(context.Background(), ReconcileInput{
			Output:  fakeOutput,
			Desired: []contracts.DesiredRecord{{Hostname: "s3.local", Answer: "10.0.0.11", Source: nextSource}},
			Visible: []contracts.VisibleRecord{{Output: provider, Hostname: "S3.local", Answer: "192.0.2.5", Provenance: &contracts.RecordProvenance{RemoteID: "rec-existing"}}},
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
		if result.Next.ManagedRecords[0].Provenance == nil || result.Next.ManagedRecords[0].Provenance.RemoteID != "rec-existing" {
			t.Fatalf("expected existing visible provenance to be retained, got %+v", result.Next.ManagedRecords[0].Provenance)
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
			Visible: []contracts.VisibleRecord{{Output: provider, Hostname: "app.local", Answer: "10.0.0.10", Provenance: &contracts.RecordProvenance{RemoteID: "rec-visible"}}},
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
		if result.Next.ManagedRecords[0].Provenance != nil {
			t.Fatalf("expected no-op keep without prior ownership proof to preserve nil provenance, got %+v", result.Next.ManagedRecords[0].Provenance)
		}
	})

	t.Run("update owned record when source identity changes but hostname stays unique", func(t *testing.T) {
		t.Parallel()

		fakeOutput := &reconcileFakeOutput{provider: provider}
		nextSource := contracts.SourceObjectRef{Provider: source.Provider, ID: "ctr-9", DisplayName: "svc-recreated"}
		owned := state.Snapshot{ManagedRecords: []state.ManagedRecord{{
			Output:     provider,
			Source:     source,
			Hostname:   "s3.local",
			Answer:     "origin.internal",
			Provenance: &contracts.RecordProvenance{RemoteID: "rec-existing"},
		}}}

		result, err := ReconcileOutput(context.Background(), ReconcileInput{
			Output:  fakeOutput,
			Desired: []contracts.DesiredRecord{{Hostname: "s3.local", Answer: "192.168.1.142", Source: nextSource}},
			Visible: []contracts.VisibleRecord{{Output: provider, Hostname: "s3.local", Answer: "legacy.internal", Provenance: &contracts.RecordProvenance{RemoteID: "rec-existing"}}},
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

	t.Run("do not update singleton visible hostname without ownership proof and retain managed state", func(t *testing.T) {
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
			Output:  fakeOutput,
			Desired: []contracts.DesiredRecord{{Hostname: "s3.local", Answer: "192.168.1.142", Source: nextSource}},
			Visible: []contracts.VisibleRecord{{Output: provider, Hostname: "s3.local", Answer: "legacy.internal", Provenance: &contracts.RecordProvenance{RemoteID: "manual-record"}}},
			Owned:   owned,
		})
		if err != nil {
			t.Fatalf("reconcile output: %v", err)
		}

		if got := len(fakeOutput.updated); got != 0 {
			t.Fatalf("expected no update calls without ownership proof, got %d", got)
		}
		if got := len(fakeOutput.created); got != 0 {
			t.Fatalf("expected no create calls when singleton hostname is already occupied, got %d", got)
		}
		if got := len(fakeOutput.deleted); got != 0 {
			t.Fatalf("expected no delete calls without ownership proof, got %d", got)
		}
		if got := len(result.Next.ManagedRecords); got != 1 {
			t.Fatalf("expected managed state to be retained without proof, got %+v", result.Next.ManagedRecords)
		}
		if result.Next.ManagedRecords[0].Source.ID != source.ID || result.Next.ManagedRecords[0].Answer != "origin.internal" {
			t.Fatalf("expected retained managed lineage to stay unchanged, got %+v", result.Next.ManagedRecords[0])
		}
	})

	t.Run("do not create desired record when same hostname is visibly occupied without owned proof", func(t *testing.T) {
		t.Parallel()

		plan := buildReconcilePlan(
			provider,
			[]contracts.DesiredRecord{{Hostname: "app.local", Answer: "10.0.0.11", Source: source}},
			[]contracts.VisibleRecord{{Output: provider, Hostname: "APP.local", Answer: "10.0.0.10", Provenance: &contracts.RecordProvenance{RemoteID: "manual-rec"}}},
			state.EmptySnapshot(),
		)

		if got := len(plan.Creates); got != 0 {
			t.Fatalf("expected no create plan while same hostname is occupied without proof, got %d", got)
		}
		if got := len(plan.Updates); got != 0 {
			t.Fatalf("expected no update plan without owned record, got %d", got)
		}
		if got := len(plan.Drops); got != 0 {
			t.Fatalf("expected no drop plan without owned record, got %d", got)
		}
	})

	t.Run("preserve owned provenance when same-key visible provenance changes", func(t *testing.T) {
		t.Parallel()

		owned := state.Snapshot{ManagedRecords: []state.ManagedRecord{{
			Output:     provider,
			Source:     source,
			Hostname:   "app.local",
			Answer:     "10.0.0.10",
			Provenance: &contracts.RecordProvenance{RemoteID: "rec-owned"},
		}}}

		plan := buildReconcilePlan(
			provider,
			[]contracts.DesiredRecord{{Hostname: "app.local", Answer: "10.0.0.10", Source: source}},
			[]contracts.VisibleRecord{{Output: provider, Hostname: "app.local", Answer: "10.0.0.10", Provenance: &contracts.RecordProvenance{RemoteID: "rec-manual"}}},
			owned,
		)

		if got := len(plan.NextManaged); got != 1 {
			t.Fatalf("expected one next managed record, got %d", got)
		}
		if plan.NextManaged[0].Provenance == nil || plan.NextManaged[0].Provenance.RemoteID != "rec-owned" {
			t.Fatalf("expected owned provenance to be preserved, got %+v", plan.NextManaged[0].Provenance)
		}
		if got := len(plan.Deletes); got != 0 {
			t.Fatalf("expected no stale delete planning for same-key provenance mismatch, got %d", got)
		}
	})

	t.Run("do not update owned record when exact-lineage visible provenance differs", func(t *testing.T) {
		t.Parallel()

		fakeOutput := &reconcileFakeOutput{provider: provider}
		owned := state.Snapshot{ManagedRecords: []state.ManagedRecord{{
			Output:     provider,
			Source:     source,
			Hostname:   "app.local",
			Answer:     "10.0.0.10",
			Provenance: &contracts.RecordProvenance{RemoteID: "rec-owned"},
		}}}

		result, err := ReconcileOutput(context.Background(), ReconcileInput{
			Output:  fakeOutput,
			Desired: []contracts.DesiredRecord{{Hostname: "app.local", Answer: "10.0.0.11", Source: source}},
			Visible: []contracts.VisibleRecord{{Output: provider, Hostname: "app.local", Answer: "10.0.0.10", Provenance: &contracts.RecordProvenance{RemoteID: "rec-manual"}}},
			Owned:   owned,
		})
		if err != nil {
			t.Fatalf("reconcile output: %v", err)
		}

		if got := len(fakeOutput.updated); got != 0 {
			t.Fatalf("expected no update calls across provenance mismatch, got %d", got)
		}
		if got := len(fakeOutput.created); got != 0 {
			t.Fatalf("expected no create calls across provenance mismatch, got %d", got)
		}
		if got := len(fakeOutput.deleted); got != 0 {
			t.Fatalf("expected no delete calls across provenance mismatch, got %d", got)
		}
		if got := len(result.Next.ManagedRecords); got != 1 {
			t.Fatalf("expected owned state to be retained non-destructively across provenance mismatch, got %+v", result.Next.ManagedRecords)
		}
		if result.Next.ManagedRecords[0].Provenance == nil || result.Next.ManagedRecords[0].Provenance.RemoteID != "rec-owned" {
			t.Fatalf("expected owned provenance to remain unchanged across provenance mismatch, got %+v", result.Next.ManagedRecords[0].Provenance)
		}
	})

	t.Run("plan stale owned delete with unique provenance proof", func(t *testing.T) {
		t.Parallel()

		owned := state.Snapshot{ManagedRecords: []state.ManagedRecord{{
			Output:     provider,
			Source:     source,
			Hostname:   "stale.local",
			Answer:     "10.0.0.12",
			Provenance: &contracts.RecordProvenance{RemoteID: "rec-stale"},
		}}}

		plan := buildReconcilePlan(
			provider,
			nil,
			[]contracts.VisibleRecord{{Output: provider, Hostname: "STALE.local", Answer: "10.0.0.12", Provenance: &contracts.RecordProvenance{RemoteID: "rec-stale"}}},
			owned,
		)

		if got := len(plan.Deletes); got != 1 {
			t.Fatalf("expected stale owned record with unique provenance proof to plan 1 delete; got %d deletes and %d drops", got, len(plan.Drops))
		}
		if got := len(plan.Drops); got != 0 {
			t.Fatalf("expected provenance-backed stale delete to avoid state-only drop, got %d drops", got)
		}
	})

	t.Run("retain stale owned records without deleting remote dns when proof is unavailable", func(t *testing.T) {
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
		if got := len(result.Next.ManagedRecords); got != 1 {
			t.Fatalf("expected stale owned state to be retained, got %d managed records", got)
		}
		if result.Next.ManagedRecords[0].Hostname != "old.local" || result.Next.ManagedRecords[0].Answer != "10.0.0.12" {
			t.Fatalf("expected retained stale owned record to stay unchanged, got %+v", result.Next.ManagedRecords[0])
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

func TestLogReconcilePlanStaleOwnedDeleteCountsRequireOwnershipProof(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "adguard", Name: "primary"}
	source := contracts.SourceObjectRef{Provider: contracts.ProviderRef{Type: "docker", Name: "local"}, ID: "ctr-1", DisplayName: "svc"}

	t.Run("provenance-backed stale owned record logs deletes=1", func(t *testing.T) {
		t.Parallel()

		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
		input := ReconcileInput{
			Output: &reconcileFakeOutput{provider: provider},
			Visible: []contracts.VisibleRecord{{
				Output:     provider,
				Hostname:   "STALE.local",
				Answer:     "10.0.0.12",
				Provenance: &contracts.RecordProvenance{RemoteID: "rec-stale"},
			}},
			Owned: state.Snapshot{ManagedRecords: []state.ManagedRecord{{
				Output:     provider,
				Source:     source,
				Hostname:   "stale.local",
				Answer:     "10.0.0.12",
				Provenance: &contracts.RecordProvenance{RemoteID: "rec-stale"},
			}}},
			Logger: logger,
		}

		logReconcilePlan(context.Background(), input, buildReconcilePlan(provider, nil, input.Visible, input.Owned))

		if got := logs.String(); !strings.Contains(got, "deletes=1") {
			t.Fatalf("expected reconcile plan log to include deletes=1, got %q", got)
		}
	})

	t.Run("same-key stale owned record without proof logs deletes=0", func(t *testing.T) {
		t.Parallel()

		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
		input := ReconcileInput{
			Output: &reconcileFakeOutput{provider: provider},
			Visible: []contracts.VisibleRecord{{
				Output:   provider,
				Hostname: "OLD.local",
				Answer:   "10.0.0.12",
			}},
			Owned: state.Snapshot{ManagedRecords: []state.ManagedRecord{{
				Output:   provider,
				Source:   source,
				Hostname: "old.local",
				Answer:   "10.0.0.12",
			}}},
			Logger: logger,
		}

		logReconcilePlan(context.Background(), input, buildReconcilePlan(provider, nil, input.Visible, input.Owned))

		if got := logs.String(); !strings.Contains(got, "deletes=0") {
			t.Fatalf("expected reconcile plan log to include deletes=0, got %q", got)
		}
	})
}

func TestStaleOwnedSameKeyWithoutOwnershipProofRetainsState(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "adguard", Name: "primary"}
	source := contracts.SourceObjectRef{Provider: contracts.ProviderRef{Type: "docker", Name: "local"}, ID: "ctr-1", DisplayName: "svc"}
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
		Visible: []contracts.VisibleRecord{
			{Output: provider, Hostname: "OLD.local", Answer: "10.0.0.12"},
			{Output: provider, Hostname: "manual.local", Answer: "10.0.0.50"},
		},
		Owned: owned,
	})
	if err != nil {
		t.Fatalf("reconcile output: %v", err)
	}

	if got := len(fakeOutput.deleted); got != 0 {
		t.Fatalf("expected no delete calls without unique ownership proof, got %d", got)
	}
	if got := len(result.Next.ManagedRecords); got != 1 {
		t.Fatalf("expected stale owned state to be retained without ownership proof, got %d managed records", got)
	}
	if result.Next.ManagedRecords[0].Hostname != "old.local" || result.Next.ManagedRecords[0].Answer != "10.0.0.12" {
		t.Fatalf("expected retained stale owned record to stay unchanged, got %+v", result.Next.ManagedRecords[0])
	}
}

func TestRetainedStaleOwnedStateAllowsLaterDisplayLineageUpdateWithoutVisibleProvenance(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "adguard", Name: "primary"}
	source := contracts.SourceObjectRef{Provider: contracts.ProviderRef{Type: "docker", Name: "local"}, ID: "ctr-1", DisplayName: "svc"}
	nextSource := contracts.SourceObjectRef{Provider: source.Provider, ID: "ctr-2", DisplayName: source.DisplayName}
	fakeOutput := &reconcileFakeOutput{provider: provider}
	owned := state.Snapshot{ManagedRecords: []state.ManagedRecord{{
		Output:   provider,
		Source:   source,
		Hostname: "whoami.test",
		Answer:   "127.0.0.1",
	}}}

	first, err := ReconcileOutput(context.Background(), ReconcileInput{
		Output:  fakeOutput,
		Desired: nil,
		Visible: []contracts.VisibleRecord{{Output: provider, Hostname: "whoami.test", Answer: "127.0.0.1"}},
		Owned:   owned,
	})
	if err != nil {
		t.Fatalf("first reconcile output: %v", err)
	}
	if got := len(first.Next.ManagedRecords); got != 1 {
		t.Fatalf("expected retained managed state across recreate gap, got %d managed records", got)
	}

	second, err := ReconcileOutput(context.Background(), ReconcileInput{
		Output:  fakeOutput,
		Desired: []contracts.DesiredRecord{{Hostname: "whoami.test", Answer: "127.0.0.2", Source: nextSource}},
		Visible: []contracts.VisibleRecord{{Output: provider, Hostname: "whoami.test", Answer: "127.0.0.1"}},
		Owned:   first.Next,
	})
	if err != nil {
		t.Fatalf("second reconcile output: %v", err)
	}
	if got := len(fakeOutput.created); got != 0 {
		t.Fatalf("expected no create calls once managed state is retained, got %d", got)
	}
	if got := len(fakeOutput.updated); got != 1 {
		t.Fatalf("expected 1 update call after recreate gap, got %d", got)
	}
	if got := len(second.Next.ManagedRecords); got != 1 {
		t.Fatalf("expected one updated managed record, got %d", got)
	}
	if second.Next.ManagedRecords[0].Source.ID != "ctr-2" || second.Next.ManagedRecords[0].Answer != "127.0.0.2" {
		t.Fatalf("expected managed state to roll forward after update, got %+v", second.Next.ManagedRecords[0])
	}
}

func TestStaleOwnedDuplicateVisibleRecordsRemainNonDestructive(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "adguard", Name: "primary"}
	source := contracts.SourceObjectRef{Provider: contracts.ProviderRef{Type: "docker", Name: "local"}, ID: "ctr-1", DisplayName: "svc"}
	fakeOutput := &reconcileFakeOutput{provider: provider}
	owned := state.Snapshot{ManagedRecords: []state.ManagedRecord{{
		Output:   provider,
		Source:   source,
		Hostname: "old.local",
		Answer:   "10.0.0.12",
	}}}

	_, err := ReconcileOutput(context.Background(), ReconcileInput{
		Output:  fakeOutput,
		Desired: nil,
		Visible: []contracts.VisibleRecord{
			{Output: provider, Hostname: "old.local", Answer: "10.0.0.12"},
			{Output: provider, Hostname: "OLD.local", Answer: "10.0.0.12"},
			{Output: provider, Hostname: "manual.local", Answer: "10.0.0.50"},
		},
		Owned: owned,
	})
	if err == nil {
		t.Fatal("expected ambiguity error for duplicate visible keys")
	}
	var ambiguityErr *ErrVisibleRecordAmbiguous
	if !errors.As(err, &ambiguityErr) {
		t.Fatalf("expected ErrVisibleRecordAmbiguous, got %T", err)
	}
	if got := len(fakeOutput.deleted); got != 0 {
		t.Fatalf("expected ambiguous stale ownership handling to remain non-destructive, got %d deletes", got)
	}
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
		Output:  fakeOutput,
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
	provider         contracts.ProviderRef
	created          []contracts.DesiredRecord
	updated          []reconcileUpdateCall
	deleted          []contracts.VisibleRecord
	createProvenance *contracts.RecordProvenance
	updateProvenance *contracts.RecordProvenance
}

func (f *reconcileFakeOutput) Provider() contracts.ProviderRef { return f.provider }

func (f *reconcileFakeOutput) ListVisible(context.Context) ([]contracts.VisibleRecord, error) {
	return nil, nil
}

func (f *reconcileFakeOutput) Create(_ context.Context, desired contracts.DesiredRecord) (*contracts.RecordProvenance, error) {
	f.created = append(f.created, desired)
	return copyRecordProvenance(f.createProvenance), nil
}

func (f *reconcileFakeOutput) Update(_ context.Context, visible contracts.VisibleRecord, desired contracts.DesiredRecord) (*contracts.RecordProvenance, error) {
	f.updated = append(f.updated, reconcileUpdateCall{From: visible, To: desired})
	return nextManagedProvenance(f.updateProvenance, visible.Provenance), nil
}

func (f *reconcileFakeOutput) Delete(_ context.Context, visible contracts.VisibleRecord) error {
	f.deleted = append(f.deleted, visible)
	return nil
}
