package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestValidateRequiresSourceAndOutput(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Sources = nil

	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "at least one source") {
		t.Fatalf("expected source validation error, got %v", err)
	}

	cfg = validConfig()
	cfg.Outputs = nil

	err = Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "at least one output") {
		t.Fatalf("expected output validation error, got %v", err)
	}
}

func TestDockerEndpointModes(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected unix socket endpoint to validate, got %v", err)
	}

	proxyCfg := decodeFixture(t, "socket-proxy.toml")
	if err := Validate(proxyCfg); err != nil {
		t.Fatalf("expected tcp proxy endpoint to validate, got %v", err)
	}

	cfg.Sources[0].Endpoint = "http://docker-proxy:2375"
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "must use unix:// or tcp://") {
		t.Fatalf("expected endpoint scheme error, got %v", err)
	}
}

func TestValidateRuntimeAndCredentialFields(t *testing.T) {
	t.Parallel()

	fixtureCfg := decodeFixture(t, "minimal.toml")
	if err := Validate(fixtureCfg); err != nil {
		t.Fatalf("expected minimal config fixture to validate, got %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name: "missing state path",
			mutate: func(cfg *Config) {
				cfg.State.Path = ""
			},
			wantErr: "state.path is required",
		},
		{
			name: "invalid log level",
			mutate: func(cfg *Config) {
				cfg.Logging.Level = "verbose"
			},
			wantErr: "logging.level must be one of",
		},
		{
			name: "trace log level is valid",
			mutate: func(cfg *Config) {
				cfg.Logging.Level = "trace"
			},
		},
		{
			name: "invalid retry duration",
			mutate: func(cfg *Config) {
				cfg.Retry.MaxInterval = "later"
			},
			wantErr: "retry.max_interval must be a valid duration",
		},
		{
			name: "missing credential source",
			mutate: func(cfg *Config) {
				cfg.Outputs[0].Password = ""
				cfg.Outputs[0].PasswordRef = ""
			},
			wantErr: "must set exactly one of password or password_ref",
		},
		{
			name: "ambiguous credential source",
			mutate: func(cfg *Config) {
				cfg.Outputs[0].PasswordRef = "ENV:ADGUARD_PASSWORD"
			},
			wantErr: "must set exactly one of password or password_ref",
		},
		{
			name: "missing username",
			mutate: func(cfg *Config) {
				cfg.Outputs[0].Username = ""
			},
			wantErr: "outputs[0].username is required",
		},
		{
			name: "invalid source host ip",
			mutate: func(cfg *Config) {
				cfg.Sources[0].HostIP = "not-an-ip"
			},
			wantErr: "sources[0].host_ip must be a valid IP address",
		},
		{
			name: "invalid source base domain",
			mutate: func(cfg *Config) {
				cfg.Sources[0].BaseDomain = "bad domain"
			},
			wantErr: "sources[0].base_domain must be a valid domain name",
		},
		{
			name: "enabled cloudflare output requires zone id",
			mutate: func(cfg *Config) {
				cfg.Outputs = append(cfg.Outputs, OutputConfig{Type: "cloudflare", Name: "primary-cloudflare", APIKey: "secret-token"})
			},
			wantErr: "outputs[1].zone_id is required",
		},
		{
			name: "enabled cloudflare output requires exactly one api key source",
			mutate: func(cfg *Config) {
				cfg.Outputs = append(cfg.Outputs, OutputConfig{Type: "cloudflare", Name: "primary-cloudflare", ZoneID: "zone-123"})
			},
			wantErr: "outputs[1] must set exactly one of api_key or api_key_ref",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(&cfg)

			err := Validate(cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected config to validate, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestResolveSecrets(t *testing.T) {
	t.Parallel()

	cfg := decodeFixture(t, "env-secret.toml")
	resolved, err := ResolveSecrets(cfg, func(name string) (string, bool) {
		if name == "ADGUARD_PASSWORD" {
			return "super-secret-value", true
		}

		return "", false
	})
	if err != nil {
		t.Fatalf("expected env-backed secret to resolve, got %v", err)
	}

	if resolved.Outputs[0].Password != "super-secret-value" {
		t.Fatalf("expected resolved password, got %q", resolved.Outputs[0].Password)
	}

	if resolved.Outputs[0].PasswordRef != "" {
		t.Fatalf("expected password_ref to be cleared after resolution, got %q", resolved.Outputs[0].PasswordRef)
	}

	cloudflareCfg := validConfig()
	cloudflareCfg.Outputs = append(cloudflareCfg.Outputs, OutputConfig{
		Type:      "cloudflare",
		Name:      "primary-cloudflare",
		ZoneID:    "zone-123",
		APIKeyRef: "ENV:CLOUDFLARE_API_KEY",
	})

	resolved, err = ResolveSecrets(cloudflareCfg, func(name string) (string, bool) {
		switch name {
		case "CLOUDFLARE_API_KEY":
			return "cloudflare-secret", true
		default:
			return "", false
		}
	})
	if err != nil {
		t.Fatalf("expected cloudflare env-backed secret to resolve, got %v", err)
	}

	if resolved.Outputs[1].APIKey != "cloudflare-secret" {
		t.Fatalf("expected resolved api key, got %q", resolved.Outputs[1].APIKey)
	}

	if resolved.Outputs[1].APIKeyRef != "" {
		t.Fatalf("expected api_key_ref to be cleared after resolution, got %q", resolved.Outputs[1].APIKeyRef)
	}

	_, err = ResolveSecrets(cfg, func(string) (string, bool) {
		return "", false
	})
	if err == nil {
		t.Fatal("expected unset environment variable error")
	}

	if strings.Contains(err.Error(), "super-secret-value") {
		t.Fatalf("secret value leaked into error: %v", err)
	}

	badCfg := validConfig()
	badCfg.Outputs[0].Password = ""
	badCfg.Outputs[0].PasswordRef = "ADGUARD_PASSWORD"

	_, err = ResolveSecrets(badCfg, func(string) (string, bool) {
		return "", false
	})
	if err == nil || !strings.Contains(err.Error(), "must start with \"ENV:\"") {
		t.Fatalf("expected ENV prefix error, got %v", err)
	}

	disabledCfg := validConfig()
	disabledCfg.Outputs = append(disabledCfg.Outputs, OutputConfig{
		Type:        "cloudflare",
		Name:        "disabled-cloudflare",
		Enabled:     boolPtr(false),
		APIKeyRef:   "ENV:MISSING_CLOUDFLARE_API_KEY",
		PasswordRef: "ENV:MISSING_PASSWORD",
	})

	resolved, err = ResolveSecrets(disabledCfg, func(string) (string, bool) {
		return "", false
	})
	if err != nil {
		t.Fatalf("expected disabled output secret refs to be skipped, got %v", err)
	}

	if resolved.Outputs[1].APIKeyRef != "ENV:MISSING_CLOUDFLARE_API_KEY" {
		t.Fatalf("expected disabled api_key_ref to remain unchanged, got %q", resolved.Outputs[1].APIKeyRef)
	}
}

func TestValidateSkipsDisabledOutputs(t *testing.T) {
	t.Parallel()

	cfg := decodeFixture(t, "dual-output-disabled.toml")
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected disabled secondary output to be ignored, got %v", err)
	}

	cfg.Outputs[1].Enabled = boolPtr(true)
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "outputs[1].zone_id is required") {
		t.Fatalf("expected enabled secondary output to fail validation, got %v", err)
	}
}

func validConfig() Config {
	return Config{
		Sources: []SourceConfig{{
			Type:     "docker",
			Name:     "local-docker",
			Endpoint: "unix:///var/run/docker.sock",
		}},
		Outputs: []OutputConfig{{
			Type:     "adguard",
			Name:     "primary-adguard",
			URL:      "http://127.0.0.1:3000",
			Username: "admin",
			Password: "inline-secret",
		}},
		State: StateConfig{Path: "./state/ownership.json"},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		Retry: RetryConfig{
			InitialInterval: "1s",
			MaxInterval:     "30s",
			MaxElapsedTime:  "5m",
		},
	}
}

func decodeFixture(t *testing.T, name string) Config {
	t.Helper()

	path := filepath.Join("..", "..", "testdata", "config", name)
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		t.Fatalf("decode fixture %s: %v", name, err)
	}

	return cfg
}

func boolPtr(value bool) *bool {
	return &value
}
