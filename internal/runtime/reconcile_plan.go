package runtime

import (
	"sort"

	"github.com/aaronlmathis/docker-dns-sync/internal/contracts"
	"github.com/aaronlmathis/docker-dns-sync/internal/state"
)

type reconcilePlan struct {
	Creates      []contracts.DesiredRecord
	Updates      []reconcileUpdateCall
	Deletes      []contracts.VisibleRecord
	Ambiguities  []*ErrVisibleRecordAmbiguous
	NextManaged  []state.ManagedRecord
	AppliedIndex map[string]state.ManagedRecord
}

func buildReconcilePlan(output contracts.ProviderRef, desired []contracts.DesiredRecord, visible []contracts.VisibleRecord, owned state.Snapshot) reconcilePlan {
	pl := reconcilePlan{AppliedIndex: make(map[string]state.ManagedRecord)}

	visibleByKey := make(map[string][]contracts.VisibleRecord)
	for _, v := range visible {
		if v.Output != output {
			continue
		}
		k := visibleRecordKey(v.Hostname, v.Answer)
		visibleByKey[k] = append(visibleByKey[k], v)
	}

	for k, records := range visibleByKey {
		if len(records) > 1 {
			pl.Ambiguities = append(pl.Ambiguities, &ErrVisibleRecordAmbiguous{Output: output.Type + "/" + output.Name, Key: k, Count: len(records)})
		}
	}

	desiredByLineage := make(map[string]contracts.DesiredRecord)
	for _, d := range desired {
		lineage := ownedLineageKey(output.Type, output.Name, d.Source.Provider.Type, d.Source.Provider.Name, d.Source.ID, d.Hostname)
		desiredByLineage[lineage] = d
	}

	ownedByLineage := make(map[string]state.ManagedRecord)
	for _, m := range owned.ManagedRecords {
		if m.Output != output {
			continue
		}
		lineage := ownedLineageKey(output.Type, output.Name, m.Source.Provider.Type, m.Source.Provider.Name, m.Source.ID, m.Hostname)
		ownedByLineage[lineage] = m
	}

	for lineage, d := range desiredByLineage {
		if ownedRecord, ok := ownedByLineage[lineage]; ok {
			oldKey := visibleRecordKey(ownedRecord.Hostname, ownedRecord.Answer)
			if len(visibleByKey[oldKey]) == 1 && visibleRecordKey(d.Hostname, d.Answer) != oldKey {
				pl.Updates = append(pl.Updates, reconcileUpdateCall{From: visibleByKey[oldKey][0], To: d})
				continue
			}
			if len(visibleByKey[oldKey]) > 1 {
				continue
			}
			pl.NextManaged = append(pl.NextManaged, state.ManagedRecord{Output: output, Source: d.Source, Hostname: d.Hostname, Answer: d.Answer})
			continue
		}

		key := visibleRecordKey(d.Hostname, d.Answer)
		if len(visibleByKey[key]) == 0 {
			pl.Creates = append(pl.Creates, d)
		}
	}

	for lineage, m := range ownedByLineage {
		if _, keep := desiredByLineage[lineage]; keep {
			continue
		}
		key := visibleRecordKey(m.Hostname, m.Answer)
		if len(visibleByKey[key]) == 1 {
			pl.Deletes = append(pl.Deletes, visibleByKey[key][0])
		}
	}

	sort.Slice(pl.Creates, func(i, j int) bool { return visibleRecordKey(pl.Creates[i].Hostname, pl.Creates[i].Answer) < visibleRecordKey(pl.Creates[j].Hostname, pl.Creates[j].Answer) })
	sort.Slice(pl.Updates, func(i, j int) bool { return visibleRecordKey(pl.Updates[i].To.Hostname, pl.Updates[i].To.Answer) < visibleRecordKey(pl.Updates[j].To.Hostname, pl.Updates[j].To.Answer) })
	sort.Slice(pl.Deletes, func(i, j int) bool { return visibleRecordKey(pl.Deletes[i].Hostname, pl.Deletes[i].Answer) < visibleRecordKey(pl.Deletes[j].Hostname, pl.Deletes[j].Answer) })

	return pl
}
