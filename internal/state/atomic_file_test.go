package state

import (
	"os"
	"path/filepath"
	"testing"
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
