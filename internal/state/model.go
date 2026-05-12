package state

import (
	"time"

	"github.com/aaronlmathis/docker-dns-sync/internal/contracts"
)

const SnapshotVersion = 1

type Snapshot struct {
	Version        int             `json:"version"`
	ManagedRecords []ManagedRecord `json:"managed_records"`
}

type ManagedRecord struct {
	Output        contracts.ProviderRef     `json:"output"`
	Source        contracts.SourceObjectRef `json:"source"`
	Hostname      string                    `json:"hostname"`
	Answer        string                    `json:"answer"`
	LastAppliedAt time.Time                 `json:"last_applied_at"`
}

func EmptySnapshot() Snapshot {
	return Snapshot{
		Version:        SnapshotVersion,
		ManagedRecords: []ManagedRecord{},
	}
}
