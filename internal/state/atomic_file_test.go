package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aaronflorey/docker-dns-sync/internal/contracts"
)

func TestAtomicWriteReplacesStateFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "ownership.json")
	if err := os.WriteFile(path, []byte("old-state\n"), 0o600); err != nil {
		t.Fatalf("seed state file: %v", err)
	}

	if err := atomicWriteFile(path, []byte("new-state\n"), 0o600); err != nil {
		t.Fatalf("atomic write: %v", err)
	}

	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}

	if string(payload) != "new-state\n" {
		t.Fatalf("expected replaced state contents, got %q", string(payload))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read state directory: %v", err)
	}

	if len(entries) != 1 || entries[0].Name() != "ownership.json" {
		t.Fatalf("expected only ownership.json to remain, got %#v", entries)
	}
}

func TestAtomicWriteCreatesNestedStateDirectory(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state", "nested", "ownership.json")
	if err := atomicWriteFile(path, []byte("new-state\n"), 0o600); err != nil {
		t.Fatalf("atomic write: %v", err)
	}

	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}

	if string(payload) != "new-state\n" {
		t.Fatalf("expected nested state contents, got %q", string(payload))
	}
}

func TestStoreLoadLegacySnapshotWithoutProvenance(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	payload := []byte(`{
  "version": 1,
  "managed_records": [
    {
      "output": {"type": "cloudflare", "name": "primary"},
      "source": {
        "provider": {"type": "docker", "name": "local"},
        "id": "ctr-1",
        "display_name": "svc"
      },
      "hostname": "app.example.com",
      "answer": "10.0.0.10",
      "last_applied_at": "2026-05-13T01:00:00Z"
    }
  ]
}`)
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write legacy snapshot: %v", err)
	}

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}

	snapshot, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(snapshot.ManagedRecords) != 1 {
		t.Fatalf("expected 1 managed record, got %d", len(snapshot.ManagedRecords))
	}
	if snapshot.ManagedRecords[0].Provenance != nil {
		t.Fatalf("expected legacy record provenance to remain nil, got %+v", snapshot.ManagedRecords[0].Provenance)
	}
}

func TestStoreSaveLoadRoundTripsProvenance(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}

	now := time.Date(2026, 5, 13, 1, 0, 0, 0, time.UTC)
	want := Snapshot{ManagedRecords: []ManagedRecord{{
		Output:   contracts.ProviderRef{Type: "cloudflare", Name: "primary"},
		Source:   contracts.SourceObjectRef{Provider: contracts.ProviderRef{Type: "docker", Name: "local"}, ID: "ctr-1", DisplayName: "svc"},
		Hostname: "app.example.com",
		Answer:   "10.0.0.10",
		Provenance: &contracts.RecordProvenance{
			RemoteID: "rec-123",
		},
		LastAppliedAt: now,
	}}}

	if err := store.Save(want); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(got.ManagedRecords) != 1 {
		t.Fatalf("expected 1 managed record, got %d", len(got.ManagedRecords))
	}
	if got.ManagedRecords[0].Provenance == nil || got.ManagedRecords[0].Provenance.RemoteID != "rec-123" {
		t.Fatalf("expected provenance remote ID rec-123, got %+v", got.ManagedRecords[0].Provenance)
	}
}
