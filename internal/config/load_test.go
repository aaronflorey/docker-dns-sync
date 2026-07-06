package config

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestLoadRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mutate      func(string) string
		wantKeyPath string
		secretValue string
	}{
		{
			name:        "top level",
			mutate:      func(tomlText string) string { return tomlText + "\nunknown_top_level = true\n" },
			wantKeyPath: "unknown_top_level",
		},
		{
			name: "source table",
			mutate: func(tomlText string) string {
				return insertAfter(t, tomlText, "endpoint = \"unix:///var/run/docker.sock\"\n", "extra_source_field = \"value\"\n")
			},
			wantKeyPath: "sources.extra_source_field",
		},
		{
			name: "output table secret-like value is redacted",
			mutate: func(tomlText string) string {
				return insertAfter(t, tomlText, "password = \"inline-secret\"\n", "unexpected_token = \"super-secret-token\"\n")
			},
			wantKeyPath: "outputs.unexpected_token",
			secretValue: "super-secret-token",
		},
		{
			name: "state table",
			mutate: func(tomlText string) string {
				return insertAfter(t, tomlText, "path = \"./state/ownership.json\"\n", "extra_state_field = true\n")
			},
			wantKeyPath: "state.extra_state_field",
		},
		{
			name: "logging table",
			mutate: func(tomlText string) string {
				return insertAfter(t, tomlText, "format = \"json\"\n", "extra_logging_field = true\n")
			},
			wantKeyPath: "logging.extra_logging_field",
		},
		{
			name: "runtime table",
			mutate: func(tomlText string) string {
				return insertAfter(t, tomlText, "operation_timeout = \"15s\"\n", "extra_runtime_field = true\n")
			},
			wantKeyPath: "runtime.extra_runtime_field",
		},
		{
			name: "retry table",
			mutate: func(tomlText string) string {
				return insertAfter(t, tomlText, "max_elapsed_time = \"5m\"\n", "extra_retry_field = true\n")
			},
			wantKeyPath: "retry.extra_retry_field",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			configPath := writeTempConfigFile(t, tt.mutate(validLoadConfigTOML(t)))

			_, err := Load(configPath)
			if err == nil {
				t.Fatal("expected load to reject unknown field")
			}

			if !strings.Contains(err.Error(), tt.wantKeyPath) {
				t.Fatalf("expected error to contain unknown key path %q, got %v", tt.wantKeyPath, err)
			}

			if tt.secretValue != "" && strings.Contains(err.Error(), tt.secretValue) {
				t.Fatalf("secret-like value leaked into error: %v", err)
			}
		})
	}
}

func TestLoadAcceptsValidFixtures(t *testing.T) {
	t.Parallel()

	fixturePaths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "config", "*.toml"))
	if err != nil {
		t.Fatalf("glob config fixtures: %v", err)
	}

	tests := []struct {
		name string
		path string
	}{
		{
			name: "root example config",
			path: filepath.Join("..", "..", "config.example.toml"),
		},
	}

	invalidFixtures := []string{"malformed.toml"}
	for _, path := range fixturePaths {
		if slices.Contains(invalidFixtures, filepath.Base(path)) {
			continue
		}

		tests = append(tests, struct {
			name string
			path string
		}{
			name: filepath.Base(path),
			path: path,
		})
	}

	for _, tt := range tests {
		ttt := tt
		t.Run(ttt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := Load(ttt.path); err != nil {
				t.Fatalf("load %s: %v", ttt.path, err)
			}
		})
	}
}

func validLoadConfigTOML(t *testing.T) string {
	t.Helper()

	cfg := validConfig()
	cfg.Runtime.OperationTimeout = "15s"

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		t.Fatalf("encode valid config: %v", err)
	}

	return buf.String()
}

func writeTempConfigFile(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	return path
}

func insertAfter(t *testing.T, contents string, needle string, addition string) string {
	t.Helper()

	if !strings.Contains(contents, needle) {
		t.Fatalf("expected config TOML to contain %q", needle)
	}

	return strings.Replace(contents, needle, needle+addition, 1)
}
