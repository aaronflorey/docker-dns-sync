package state

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	createdDirs, err := missingDirChain(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	for _, createdDir := range createdDirs {
		if err := syncDir(filepath.Dir(createdDir)); err != nil {
			return err
		}
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp state file: %w", err)
	}

	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp state file: %w", err)
	}

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp state file: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp state file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp state file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp state file: %w", err)
	}
	if err := syncDir(dir); err != nil {
		return err
	}

	cleanup = false
	return nil
}

func missingDirChain(path string) ([]string, error) {
	missing := make([]string, 0)
	current := path
	for {
		_, err := os.Stat(current)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat state directory: %w", err)
		}

		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	slices.Reverse(missing)
	return missing, nil
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open state directory: %w", err)
	}
	defer dir.Close()

	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync state directory: %w", err)
	}

	return nil
}
