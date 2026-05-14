package runtime

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/aaronlmathis/docker-dns-sync/internal/contracts"
	"github.com/aaronlmathis/docker-dns-sync/internal/state"
)

func applyReconcilePlan(ctx context.Context, output contracts.Output, owned state.Snapshot, plan reconcilePlan, now time.Time) (state.Snapshot, bool, error) {
	recordsByLineage := make(map[string]state.ManagedRecord)
	otherRecords := make([]state.ManagedRecord, 0, len(owned.ManagedRecords))
	for _, record := range owned.ManagedRecords {
		if record.Output != output.Provider() {
			otherRecords = append(otherRecords, record)
			continue
		}
		recordsByLineage[managedRecordLineageKey(record)] = record
	}

	progressed := false

	for _, drop := range plan.Drops {
		delete(recordsByLineage, managedRecordLineageKey(drop))
	}

	for _, create := range plan.Creates {
		if err := output.Create(ctx, create); err != nil {
			return snapshotWithOutputRecords(otherRecords, recordsByLineage), progressed, fmt.Errorf("create %s: %w", visibleRecordKey(create.Hostname, create.Answer), err)
		}
		recordsByLineage[desiredRecordLineageKey(output.Provider(), create)] = state.ManagedRecord{Output: output.Provider(), Source: create.Source, Hostname: create.Hostname, Answer: create.Answer, LastAppliedAt: now}
		progressed = true
	}

	for _, update := range plan.Updates {
		if err := output.Update(ctx, update.From, update.To); err != nil {
			return snapshotWithOutputRecords(otherRecords, recordsByLineage), progressed, fmt.Errorf("update %s: %w", visibleRecordKey(update.From.Hostname, update.From.Answer), err)
		}
		recordsByLineage[desiredRecordLineageKey(output.Provider(), update.To)] = state.ManagedRecord{Output: output.Provider(), Source: update.To.Source, Hostname: update.To.Hostname, Answer: update.To.Answer, LastAppliedAt: now}
		progressed = true
	}

	for _, del := range plan.Deletes {
		if err := output.Delete(ctx, del.Visible); err != nil {
			return snapshotWithOutputRecords(otherRecords, recordsByLineage), progressed, fmt.Errorf("delete %s: %w", visibleRecordKey(del.Visible.Hostname, del.Visible.Answer), err)
		}
		delete(recordsByLineage, managedRecordLineageKey(del.Managed))
		progressed = true
	}

	for _, keep := range plan.NextManaged {
		keep.LastAppliedAt = now
		recordsByLineage[managedRecordLineageKey(keep)] = keep
	}

	return snapshotWithOutputRecords(otherRecords, recordsByLineage), progressed, nil
}

func snapshotWithOutputRecords(otherRecords []state.ManagedRecord, recordsByLineage map[string]state.ManagedRecord) state.Snapshot {
	next := state.EmptySnapshot()
	next.ManagedRecords = append(next.ManagedRecords, otherRecords...)

	lineages := make([]string, 0, len(recordsByLineage))
	for lineage := range recordsByLineage {
		lineages = append(lineages, lineage)
	}
	sort.Strings(lineages)

	for _, lineage := range lineages {
		next.ManagedRecords = append(next.ManagedRecords, recordsByLineage[lineage])
	}

	return next
}

func desiredRecordLineageKey(output contracts.ProviderRef, desired contracts.DesiredRecord) string {
	return ownedLineageKey(output.Type, output.Name, desired.Source.Provider.Type, desired.Source.Provider.Name, desired.Source.ID, desired.Hostname)
}

func managedRecordLineageKey(record state.ManagedRecord) string {
	return ownedLineageKey(record.Output.Type, record.Output.Name, record.Source.Provider.Type, record.Source.Provider.Name, record.Source.ID, record.Hostname)
}
