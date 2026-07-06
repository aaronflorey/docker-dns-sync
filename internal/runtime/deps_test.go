package runtime

import (
	"context"
	"testing"
	"time"
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

func TestNewRuntimeDepsDefaultsOperationTimeout(t *testing.T) {
	t.Parallel()

	deps, err := NewRuntimeDeps(testRuntimeConfig(t.TempDir()))
	if err != nil {
		t.Fatalf("NewRuntimeDeps returned error: %v", err)
	}

	if deps.OperationTimeout != defaultOperationTimeout {
		t.Fatalf("expected default operation timeout %v, got %v", defaultOperationTimeout, deps.OperationTimeout)
	}
}

func TestNewRuntimeDepsUsesConfiguredOperationTimeout(t *testing.T) {
	t.Parallel()

	cfg := testRuntimeConfig(t.TempDir())
	cfg.Runtime.OperationTimeout = "25s"

	deps, err := NewRuntimeDeps(cfg)
	if err != nil {
		t.Fatalf("NewRuntimeDeps returned error: %v", err)
	}

	if deps.OperationTimeout != 25*time.Second {
		t.Fatalf("expected configured operation timeout %v, got %v", 25*time.Second, deps.OperationTimeout)
	}
}
