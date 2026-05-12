package state

import (
	"encoding/json"
	"fmt"
	"os"
)

type Store struct {
	path string
}

func NewStore(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("state path is required")
	}

	store := &Store{path: path}
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat state file: %w", err)
		}

		if err := store.Save(EmptySnapshot()); err != nil {
			return nil, err
		}
	}

	if _, err := store.Load(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) Load() (Snapshot, error) {
	payload, err := os.ReadFile(s.path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read state file: %w", err)
	}

	var snapshot Snapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("decode state file: %w", err)
	}

	if snapshot.Version != SnapshotVersion {
		return Snapshot{}, fmt.Errorf("unsupported state snapshot version %d", snapshot.Version)
	}

	if snapshot.ManagedRecords == nil {
		snapshot.ManagedRecords = []ManagedRecord{}
	}

	return snapshot, nil
}

func (s *Store) Save(snapshot Snapshot) error {
	snapshot.Version = SnapshotVersion
	if snapshot.ManagedRecords == nil {
		snapshot.ManagedRecords = []ManagedRecord{}
	}

	payload, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state snapshot: %w", err)
	}

	payload = append(payload, '\n')
	if err := atomicWriteFile(s.path, payload, 0o600); err != nil {
		return err
	}

	return nil
}
