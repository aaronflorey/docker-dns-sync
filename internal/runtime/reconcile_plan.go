package runtime

import (
	"sort"

	"github.com/aaronflorey/docker-dns-sync/internal/contracts"
	"github.com/aaronflorey/docker-dns-sync/internal/state"
)

type reconcilePlan struct {
	Creates      []contracts.DesiredRecord
	Updates      []reconcileUpdateCall
	Deletes      []reconcileDeleteCall
	Drops        []state.ManagedRecord
	CreateDrops  map[string]state.ManagedRecord
	UpdateDrops  map[string]state.ManagedRecord
	KeepDrops    map[string]state.ManagedRecord
	Ambiguities  []*ErrVisibleRecordAmbiguous
	NextManaged  []state.ManagedRecord
	AppliedIndex map[string]state.ManagedRecord
}

type reconcileDeleteCall struct {
	Visible contracts.VisibleRecord
	Managed state.ManagedRecord
}

func buildReconcilePlan(output contracts.ProviderRef, desired []contracts.DesiredRecord, visible []contracts.VisibleRecord, owned state.Snapshot) reconcilePlan {
	pl := reconcilePlan{
		AppliedIndex: make(map[string]state.ManagedRecord),
		CreateDrops:  make(map[string]state.ManagedRecord),
		UpdateDrops:  make(map[string]state.ManagedRecord),
		KeepDrops:    make(map[string]state.ManagedRecord),
	}

	visibleByKey := make(map[string][]contracts.VisibleRecord)
	visibleByHostname := make(map[string][]contracts.VisibleRecord)
	for _, v := range visible {
		if v.Output != output {
			continue
		}
		k := visibleRecordKey(v.Hostname, v.Answer)
		visibleByKey[k] = append(visibleByKey[k], v)
		hostname := normalizeHostname(v.Hostname)
		visibleByHostname[hostname] = append(visibleByHostname[hostname], v)
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
	ownedByDisplayLineage := make(map[string][]state.ManagedRecord)
	ownedByHostname := make(map[string][]state.ManagedRecord)
	for _, m := range owned.ManagedRecords {
		if m.Output != output {
			continue
		}
		lineage := ownedLineageKey(output.Type, output.Name, m.Source.Provider.Type, m.Source.Provider.Name, m.Source.ID, m.Hostname)
		ownedByLineage[lineage] = m
		displayLineage := ownedDisplayLineageKey(output, m.Source, m.Hostname)
		ownedByDisplayLineage[displayLineage] = append(ownedByDisplayLineage[displayLineage], m)
		hostname := normalizeHostname(m.Hostname)
		ownedByHostname[hostname] = append(ownedByHostname[hostname], m)
	}
	matchedOwnedLineages := make(map[string]struct{}, len(ownedByLineage))

	for lineage, d := range desiredByLineage {
		matchedByHostnameOnly := false
		ownedRecord, ok := ownedByLineage[lineage]
		if !ok {
			candidates := ownedByDisplayLineage[ownedDisplayLineageKey(output, d.Source, d.Hostname)]
			if len(candidates) == 1 {
				ownedRecord = candidates[0]
				ok = true
			}
		}
		if !ok {
			candidates := ownedByHostname[normalizeHostname(d.Hostname)]
			if len(candidates) == 1 {
				ownedRecord = candidates[0]
				ok = true
				matchedByHostnameOnly = true
			}
		}
		if ok {
			ownedLineage := managedRecordLineageKey(ownedRecord)
			matchedOwnedLineages[ownedLineage] = struct{}{}
			hostnameMatches := visibleByHostname[normalizeHostname(d.Hostname)]
			oldKey := visibleRecordKey(ownedRecord.Hostname, ownedRecord.Answer)
			newKey := visibleRecordKey(d.Hostname, d.Answer)
			if len(visibleByKey[oldKey]) == 1 && visibleRecordKey(d.Hostname, d.Answer) != oldKey {
				if canUpdateOwnedVisibleRecord(ownedRecord, visibleByKey[oldKey][0], matchedByHostnameOnly) {
					if ownedLineage != lineage {
						pl.UpdateDrops[lineage] = ownedRecord
					}
					pl.Updates = append(pl.Updates, reconcileUpdateCall{From: visibleByKey[oldKey][0], To: d})
					continue
				}
				continue
			}
			if len(visibleByKey[oldKey]) > 1 {
				continue
			}
			if len(hostnameMatches) == 1 && newKey != visibleRecordKey(hostnameMatches[0].Hostname, hostnameMatches[0].Answer) {
				if canUpdateOwnedVisibleRecord(ownedRecord, hostnameMatches[0], matchedByHostnameOnly) {
					if ownedLineage != lineage {
						pl.UpdateDrops[lineage] = ownedRecord
					}
					pl.Updates = append(pl.Updates, reconcileUpdateCall{From: hostnameMatches[0], To: d})
					continue
				}
				continue
			}
			if len(visibleByKey[newKey]) == 0 {
				if len(hostnameMatches) > 0 {
					continue
				}
				if ownedLineage != lineage {
					pl.CreateDrops[lineage] = ownedRecord
				}
				pl.Creates = append(pl.Creates, d)
				continue
			}
			provenance := copyRecordProvenance(ownedRecord.Provenance)
			if sameRecordProvenance(ownedRecord.Provenance, visibleByKey[newKey][0].Provenance) {
				provenance = copyRecordProvenance(visibleByKey[newKey][0].Provenance)
			}
			if ownedLineage != lineage {
				pl.KeepDrops[lineage] = ownedRecord
			}
			pl.NextManaged = append(pl.NextManaged, state.ManagedRecord{
				Output:     output,
				Source:     d.Source,
				Hostname:   d.Hostname,
				Answer:     d.Answer,
				Provenance: provenance,
			})
			continue
		}

		key := visibleRecordKey(d.Hostname, d.Answer)
		if len(visibleByKey[key]) == 0 && len(visibleByHostname[normalizeHostname(d.Hostname)]) == 0 {
			pl.Creates = append(pl.Creates, d)
		}
	}

	for lineage, m := range ownedByLineage {
		if _, matched := matchedOwnedLineages[lineage]; matched {
			continue
		}
		if _, keep := desiredByLineage[lineage]; keep {
			continue
		}
		if visible, ok := staleOwnedDeleteVisibleRecord(m, visibleByKey); ok {
			pl.Deletes = append(pl.Deletes, reconcileDeleteCall{Visible: visible, Managed: m})
			continue
		}
		if shouldRetainManagedRecordWithoutProof(m, visibleByHostname) {
			pl.NextManaged = append(pl.NextManaged, m)
			continue
		}
		pl.Drops = append(pl.Drops, m)
	}

	sort.Slice(pl.Creates, func(i, j int) bool {
		return visibleRecordKey(pl.Creates[i].Hostname, pl.Creates[i].Answer) < visibleRecordKey(pl.Creates[j].Hostname, pl.Creates[j].Answer)
	})
	sort.Slice(pl.Updates, func(i, j int) bool {
		return visibleRecordKey(pl.Updates[i].To.Hostname, pl.Updates[i].To.Answer) < visibleRecordKey(pl.Updates[j].To.Hostname, pl.Updates[j].To.Answer)
	})
	sort.Slice(pl.Deletes, func(i, j int) bool {
		return visibleRecordKey(pl.Deletes[i].Visible.Hostname, pl.Deletes[i].Visible.Answer) < visibleRecordKey(pl.Deletes[j].Visible.Hostname, pl.Deletes[j].Visible.Answer)
	})

	return pl
}

func staleOwnedDeleteVisibleRecord(managed state.ManagedRecord, visibleByKey map[string][]contracts.VisibleRecord) (contracts.VisibleRecord, bool) {
	key := visibleRecordKey(managed.Hostname, managed.Answer)
	matches := visibleByKey[key]
	if len(matches) != 1 {
		return contracts.VisibleRecord{}, false
	}
	if !sameRecordProvenance(managed.Provenance, matches[0].Provenance) {
		return contracts.VisibleRecord{}, false
	}

	return matches[0], true
}

func shouldRetainManagedRecordWithoutProof(managed state.ManagedRecord, visibleByHostname map[string][]contracts.VisibleRecord) bool {
	return len(visibleByHostname[normalizeHostname(managed.Hostname)]) > 0
}

func canUpdateOwnedVisibleRecord(owned state.ManagedRecord, visible contracts.VisibleRecord, matchedByHostnameOnly bool) bool {
	if hasRecordProvenance(visible.Provenance) {
		return sameRecordProvenance(owned.Provenance, visible.Provenance)
	}

	if !matchedByHostnameOnly {
		return true
	}

	return false
}

func hasRecordProvenance(provenance *contracts.RecordProvenance) bool {
	return provenance != nil && provenance.RemoteID != ""
}

func sameRecordProvenance(managed, visible *contracts.RecordProvenance) bool {
	if managed == nil || visible == nil {
		return false
	}
	if managed.RemoteID == "" || visible.RemoteID == "" {
		return false
	}

	return managed.RemoteID == visible.RemoteID
}

func nextManagedProvenance(primary, fallback *contracts.RecordProvenance) *contracts.RecordProvenance {
	if primary != nil {
		copy := *primary
		return &copy
	}
	if fallback != nil {
		copy := *fallback
		return &copy
	}

	return nil
}
