package runtime

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/aaronflorey/docker-dns-sync/internal/contracts"
	"github.com/aaronflorey/docker-dns-sync/internal/state"
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

	for _, keep := range plan.NextManaged {
		keep.LastAppliedAt = now
		lineage := managedRecordLineageKey(keep)
		recordsByLineage[lineage] = keep
		if drop, ok := plan.KeepDrops[lineage]; ok {
			delete(recordsByLineage, managedRecordLineageKey(drop))
		}
	}

	for _, create := range plan.Creates {
		provenance, err := output.Create(ctx, create)
		if err != nil {
			return snapshotWithOutputRecords(otherRecords, recordsByLineage), progressed, fmt.Errorf("create %s: %w", visibleRecordKey(create.Hostname, create.Answer), err)
		}
		lineage := desiredRecordLineageKey(output.Provider(), create)
		recordsByLineage[lineage] = state.ManagedRecord{Output: output.Provider(), Source: create.Source, Hostname: create.Hostname, Answer: create.Answer, Provenance: copyRecordProvenance(provenance), LastAppliedAt: now}
		if drop, ok := plan.CreateDrops[lineage]; ok {
			delete(recordsByLineage, managedRecordLineageKey(drop))
		}
		progressed = true
	}

	for _, update := range plan.Updates {
		provenance, err := output.Update(ctx, update.From, update.To)
		if err != nil {
			return snapshotWithOutputRecords(otherRecords, recordsByLineage), progressed, fmt.Errorf("update %s: %w", visibleRecordKey(update.From.Hostname, update.From.Answer), err)
		}
		lineage := desiredRecordLineageKey(output.Provider(), update.To)
		recordsByLineage[lineage] = state.ManagedRecord{Output: output.Provider(), Source: update.To.Source, Hostname: update.To.Hostname, Answer: update.To.Answer, Provenance: nextManagedProvenance(provenance, update.From.Provenance), LastAppliedAt: now}
		if drop, ok := plan.UpdateDrops[lineage]; ok {
			delete(recordsByLineage, managedRecordLineageKey(drop))
		}
		progressed = true
	}

	for _, del := range plan.Deletes {
		// Delete the visible output record before removing its managed lineage so
		// persistence only forgets ownership after the external mutation succeeds.
		if err := output.Delete(ctx, del.Visible); err != nil {
			return snapshotWithOutputRecords(otherRecords, recordsByLineage), progressed, fmt.Errorf("delete %s: %w", visibleRecordKey(del.Visible.Hostname, del.Visible.Answer), err)
		}
		delete(recordsByLineage, managedRecordLineageKey(del.Managed))
		progressed = true
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

func copyRecordProvenance(provenance *contracts.RecordProvenance) *contracts.RecordProvenance {
	if provenance == nil {
		return nil
	}

	copy := *provenance
	return &copy
}
