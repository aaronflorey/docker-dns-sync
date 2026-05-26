package runtime

import (
	"context"
	"testing"
)

func TestParseLogLevelSupportsTrace(t *testing.T) {
	t.Parallel()

	level, err := parseLogLevel("trace")
	if err != nil {
		t.Fatalf("parseLogLevel returned error: %v", err)
	}
	if level != LevelTrace {
		t.Fatalf("expected trace level %v, got %v", LevelTrace, level)
	}
}

func TestNewRuntimeDepsInfoDisablesTrace(t *testing.T) {
	t.Parallel()

	deps, err := NewRuntimeDeps(testRuntimeConfig(t.TempDir()))
	if err != nil {
		t.Fatalf("NewRuntimeDeps returned error: %v", err)
	}
	if deps.Logger.Enabled(context.Background(), LevelTrace) {
		t.Fatal("expected info logger to disable trace level")
	}
}
