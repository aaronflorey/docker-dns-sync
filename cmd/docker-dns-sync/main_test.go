package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aaronflorey/docker-dns-sync/internal/config"
	"github.com/aaronflorey/docker-dns-sync/internal/runtime"
)

func TestRunWithMinimalConfig(t *testing.T) {
	t.Parallel()

	dockerServer := newDockerServer(t)
	configPath := tempConfigPath(t, "minimal.toml", dockerServer.URL, newAdGuardServer(t).URL)
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

	dockerServer := newDockerServer(t)
	configPath := tempConfigPath(t, "minimal.toml", dockerServer.URL, newAdGuardServer(t).URL)
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

func TestRunInitializesStateStoreAndProviders(t *testing.T) {
	dockerServer := newDockerServer(t)
	configPath := tempConfigPath(t, "minimal.toml", dockerServer.URL, newAdGuardServer(t).URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var captured *runtime.App
	restore := swapNewApp(func(cfg config.Config) appRunner {
		captured = runtime.New(cfg)
		return captured
	})
	defer restore()

	done := make(chan error, 1)
	go func() {
		done <- runWithContext(ctx, []string{"-config", configPath})
	}()

	waitFor(t, func() bool {
		return captured != nil && captured.StateStore() != nil && captured.SourceCount() == 1 && captured.OutputCount() == 1
	})

	if _, err := os.Stat(captured.StateStore().Path()); err != nil {
		t.Fatalf("expected state file to exist: %v", err)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected clean shutdown, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for shutdown")
	}
}

func TestRuntimeAppliesLoggingAndRetryConfig(t *testing.T) {
	dockerServer := newDockerServer(t)
	configPath := tempConfigPath(t, "minimal.toml", dockerServer.URL, newAdGuardServer(t).URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var captured *runtime.App
	restore := swapNewApp(func(cfg config.Config) appRunner {
		captured = runtime.New(cfg)
		return captured
	})
	defer restore()

	done := make(chan error, 1)
	go func() {
		done <- runWithContext(ctx, []string{"-config", configPath})
	}()

	waitFor(t, func() bool {
		return captured != nil && captured.StateStore() != nil
	})

	deps := captured.Deps()
	if deps.LogLevel != slog.LevelInfo {
		t.Fatalf("expected log level info, got %v", deps.LogLevel)
	}

	if !deps.Logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("expected info level to be enabled")
	}

	if deps.Logger.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("expected debug level to be disabled for info logger")
	}

	if deps.Retry.InitialInterval != time.Second {
		t.Fatalf("expected retry initial interval 1s, got %v", deps.Retry.InitialInterval)
	}

	if deps.Retry.MaxInterval != 30*time.Second {
		t.Fatalf("expected retry max interval 30s, got %v", deps.Retry.MaxInterval)
	}

	if deps.Retry.MaxElapsedTime != 5*time.Minute {
		t.Fatalf("expected retry max elapsed time 5m, got %v", deps.Retry.MaxElapsedTime)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected clean shutdown, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for shutdown")
	}
}

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", "config", name)
}

func tempConfigPath(t *testing.T, name string, dockerURL string, adguardURL string) string {
	t.Helper()

	payload, err := os.ReadFile(fixturePath(t, name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	statePath := filepath.Join(t.TempDir(), "state", "ownership.json")
	rewritten := strings.ReplaceAll(string(payload), "./state/ownership.json", statePath)
	rewritten = strings.ReplaceAll(rewritten, "unix:///var/run/docker.sock", strings.Replace(dockerURL, "http://", "tcp://", 1))
	rewritten = strings.ReplaceAll(rewritten, "http://127.0.0.1:3000", adguardURL)
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(rewritten), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	return path
}

func newDockerServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/containers/json") || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	t.Cleanup(server.Close)
	return server
}

func newAdGuardServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/control/rewrite/list" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	t.Cleanup(server.Close)
	return server
}

func swapNewApp(factory func(config.Config) appRunner) func() {
	previous := newApp
	newApp = factory
	return func() {
		newApp = previous
	}
}

func waitFor(t *testing.T, ready func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal(fmt.Sprintf("condition was not met before timeout"))
}
