package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/aaronflorey/docker-dns-sync/internal/contracts"
	"github.com/aaronflorey/docker-dns-sync/internal/state"
)

type ReconcileInput struct {
	Output  contracts.Output
	Desired []contracts.DesiredRecord
	Visible []contracts.VisibleRecord
	Owned   state.Snapshot
	Now     func() time.Time
	Logger  *slog.Logger
}

type ReconcileResult struct {
	Next        state.Snapshot
	Ambiguities []*ErrVisibleRecordAmbiguous
}

type outputMutationError struct {
	provider contracts.ProviderRef
	err      error
}

func (e outputMutationError) Error() string {
	return e.err.Error()
}

func (e outputMutationError) Unwrap() error {
	return e.err
}

type reconcileUpdateCall struct {
	From contracts.VisibleRecord
	To   contracts.DesiredRecord
}

type partialMutationProgressError struct {
	err error
}

func (e partialMutationProgressError) Error() string {
	return e.err.Error()
}

func (e partialMutationProgressError) Unwrap() error {
	return e.err
}

func ReconcileOutput(ctx context.Context, input ReconcileInput) (ReconcileResult, error) {
	result, _, err := reconcileOutput(ctx, input)
	return result, err
}

func reconcileOutput(ctx context.Context, input ReconcileInput) (ReconcileResult, bool, error) {
	nowFn := input.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	plan := buildReconcilePlan(input.Output.Provider(), input.Desired, input.Visible, input.Owned)
	if len(plan.Ambiguities) > 0 {
		return ReconcileResult{Ambiguities: plan.Ambiguities}, false, plan.Ambiguities[0]
	}

	next, progressed, err := applyReconcilePlan(ctx, input.Output, input.Owned, plan, nowFn().UTC())
	if err != nil {
		wrappedErr := outputMutationError{provider: input.Output.Provider(), err: err}
		if progressed {
			return ReconcileResult{Next: next, Ambiguities: plan.Ambiguities}, true, partialMutationProgressError{err: wrappedErr}
		}
		return ReconcileResult{}, false, wrappedErr
	}

	return ReconcileResult{Next: next, Ambiguities: plan.Ambiguities}, false, nil
}

func ReconcileAndPersist(ctx context.Context, store *state.Store, input ReconcileInput) (ReconcileResult, error) {
	result, progressed, err := reconcileOutput(ctx, input)
	if err != nil {
		var partialErr partialMutationProgressError
		if progressed && errors.As(err, &partialErr) {
			if input.Logger != nil {
				input.Logger.Warn("persisting partial state snapshot after mutation failure", "output", providerKey(input.Output.Provider()), "records", len(result.Next.ManagedRecords), "error", partialErr)
			}
			if saveErr := store.Save(result.Next); saveErr != nil {
				return ReconcileResult{}, errors.Join(err, fmt.Errorf("persist partial state snapshot: %w", saveErr))
			}
			if input.Logger != nil {
				input.Logger.Warn("persisted partial state snapshot after mutation failure", "output", providerKey(input.Output.Provider()), "records", len(result.Next.ManagedRecords))
			}
		}
		return ReconcileResult{}, err
	}

	if input.Logger != nil {
		input.Logger.Info("persisting state snapshot", "output", providerKey(input.Output.Provider()), "records", len(result.Next.ManagedRecords))
	}

	if err := store.Save(result.Next); err != nil {
		return ReconcileResult{}, err
	}

	if input.Logger != nil {
		input.Logger.Info("persisted state snapshot", "output", providerKey(input.Output.Provider()), "records", len(result.Next.ManagedRecords))
	}

	return result, nil
}
