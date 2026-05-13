package runtime

import (
	"context"
	"time"

	"github.com/aaronlmathis/docker-dns-sync/internal/contracts"
	"github.com/aaronlmathis/docker-dns-sync/internal/state"
)

type ReconcileInput struct {
	Output  contracts.Output
	Desired []contracts.DesiredRecord
	Visible []contracts.VisibleRecord
	Owned   state.Snapshot
	Now     func() time.Time
}

type ReconcileResult struct {
	Next        state.Snapshot
	Ambiguities []*ErrVisibleRecordAmbiguous
}

type reconcileUpdateCall struct {
	From contracts.VisibleRecord
	To   contracts.DesiredRecord
}

func ReconcileOutput(ctx context.Context, input ReconcileInput) (ReconcileResult, error) {
	nowFn := input.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	plan := buildReconcilePlan(input.Output.Provider(), input.Desired, input.Visible, input.Owned)
	if len(plan.Ambiguities) > 0 {
		return ReconcileResult{Ambiguities: plan.Ambiguities}, plan.Ambiguities[0]
	}

	next, err := applyReconcilePlan(ctx, input.Output, plan, nowFn().UTC())
	if err != nil {
		return ReconcileResult{}, err
	}

	return ReconcileResult{Next: next, Ambiguities: plan.Ambiguities}, nil
}

func ReconcileAndPersist(ctx context.Context, store *state.Store, input ReconcileInput) (ReconcileResult, error) {
	result, err := ReconcileOutput(ctx, input)
	if err != nil {
		return ReconcileResult{}, err
	}

	if err := store.Save(result.Next); err != nil {
		return ReconcileResult{}, err
	}

	return result, nil
}
