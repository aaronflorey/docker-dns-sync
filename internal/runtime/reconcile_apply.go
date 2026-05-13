package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/aaronlmathis/docker-dns-sync/internal/contracts"
	"github.com/aaronlmathis/docker-dns-sync/internal/state"
)

func applyReconcilePlan(ctx context.Context, output contracts.Output, plan reconcilePlan, now time.Time) (state.Snapshot, error) {
	next := state.EmptySnapshot()

	for _, create := range plan.Creates {
		if err := output.Create(ctx, create); err != nil {
			return state.Snapshot{}, fmt.Errorf("create %s: %w", visibleRecordKey(create.Hostname, create.Answer), err)
		}
		next.ManagedRecords = append(next.ManagedRecords, state.ManagedRecord{Output: output.Provider(), Source: create.Source, Hostname: create.Hostname, Answer: create.Answer, LastAppliedAt: now})
	}

	for _, update := range plan.Updates {
		if err := output.Update(ctx, update.From, update.To); err != nil {
			return state.Snapshot{}, fmt.Errorf("update %s: %w", visibleRecordKey(update.From.Hostname, update.From.Answer), err)
		}
		next.ManagedRecords = append(next.ManagedRecords, state.ManagedRecord{Output: output.Provider(), Source: update.To.Source, Hostname: update.To.Hostname, Answer: update.To.Answer, LastAppliedAt: now})
	}

	for _, keep := range plan.NextManaged {
		keep.LastAppliedAt = now
		next.ManagedRecords = append(next.ManagedRecords, keep)
	}

	for _, del := range plan.Deletes {
		if err := output.Delete(ctx, del); err != nil {
			return state.Snapshot{}, fmt.Errorf("delete %s: %w", visibleRecordKey(del.Hostname, del.Answer), err)
		}
	}

	return next, nil
}
