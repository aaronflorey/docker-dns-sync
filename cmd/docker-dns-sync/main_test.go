package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunWithMinimalConfig(t *testing.T) {
	t.Parallel()

	configPath := fixturePath(t, "minimal.toml")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runWithContext(ctx, []string{"-config", configPath})
	}()

	select {
	case err := <-done:
		t.Fatalf("expected run loop to block until cancelled, got early return: %v", err)
	case <-time.After(100 * time.Millisecond):
		// expected: run loop is alive
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected clean shutdown, got error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for shutdown")
	}
}

func TestRunRejectsMissingConfig(t *testing.T) {
	t.Parallel()

	err := run(nil)
	if err == nil {
		t.Fatal("expected error for missing -config")
	}

	if !strings.Contains(err.Error(), "-config is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunRejectsMalformedConfig(t *testing.T) {
	t.Parallel()

	err := run([]string{"-config", fixturePath(t, "malformed.toml")})
	if err == nil {
		t.Fatal("expected malformed config error")
	}

	if !strings.Contains(err.Error(), "decode config file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunBlocksUntilCancelled(t *testing.T) {
	t.Parallel()

	configPath := fixturePath(t, "minimal.toml")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)

	go func() {
		done <- runWithContext(ctx, []string{"-config", configPath})
	}()

	select {
	case <-done:
		t.Fatal("run returned before cancellation")
	case <-time.After(150 * time.Millisecond):
		// expected
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run should return nil on cancellation, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run did not return after cancellation")
	}
}

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", "config", name)
}
