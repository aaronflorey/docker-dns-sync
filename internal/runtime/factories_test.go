package runtime

import (
	"context"
	"testing"

	"github.com/aaronlmathis/docker-dns-sync/internal/config"
	"github.com/aaronlmathis/docker-dns-sync/internal/contracts"
	adguardstub "github.com/aaronlmathis/docker-dns-sync/internal/providers/adguardstub"
	dockerprovider "github.com/aaronlmathis/docker-dns-sync/internal/providers/docker"
)

func TestFactoryRegistryExtensibility(t *testing.T) {
	t.Parallel()

	registry := NewFactoryRegistry()
	if err := registry.RegisterSource("fake-source", func(cfg config.SourceConfig, _ RuntimeDeps) (contracts.Source, error) {
		return fakeSource{provider: contracts.ProviderRef{Type: cfg.Type, Name: cfg.Name}}, nil
	}); err != nil {
		t.Fatalf("register source: %v", err)
	}

	if err := registry.RegisterOutput("fake-output", func(cfg config.OutputConfig, _ RuntimeDeps) (contracts.Output, error) {
		return fakeOutput{provider: contracts.ProviderRef{Type: cfg.Type, Name: cfg.Name}}, nil
	}); err != nil {
		t.Fatalf("register output: %v", err)
	}

	sources, outputs, err := registry.BuildProviders(config.Config{
		Sources: []config.SourceConfig{{Type: "fake-source", Name: "source-a", Endpoint: "unix:///var/run/docker.sock"}},
		Outputs: []config.OutputConfig{{Type: "fake-output", Name: "output-a", URL: "http://127.0.0.1:3000", Username: "admin", Password: "secret"}},
	}, RuntimeDeps{})
	if err != nil {
		t.Fatalf("build providers: %v", err)
	}

	if got := sources[0].Provider(); got.Type != "fake-source" || got.Name != "source-a" {
		t.Fatalf("unexpected source provider: %+v", got)
	}

	if got := outputs[0].Provider(); got.Type != "fake-output" || got.Name != "output-a" {
		t.Fatalf("unexpected output provider: %+v", got)
	}
}

func TestBuildProvidersFromConfig(t *testing.T) {
	t.Parallel()

	registry := NewDefaultFactoryRegistry()
	sources, outputs, err := registry.BuildProviders(validRuntimeConfig("unix:///var/run/docker.sock"), RuntimeDeps{})
	if err != nil {
		t.Fatalf("build providers: %v", err)
	}

	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}

	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}

	if _, ok := sources[0].(*dockerprovider.Provider); !ok {
		t.Fatalf("expected docker provider, got %T", sources[0])
	}

	if _, ok := outputs[0].(*adguardstub.Provider); !ok {
		t.Fatalf("expected adguard stub provider, got %T", outputs[0])
	}
}

func TestDockerSourceUsesConfiguredEndpoint(t *testing.T) {
	t.Parallel()

	registry := NewDefaultFactoryRegistry()
	for _, endpoint := range []string{"unix:///var/run/docker.sock", "tcp://docker-socket-proxy:2375"} {
		endpoint := endpoint
		t.Run(endpoint, func(t *testing.T) {
			t.Parallel()

			sources, err := registry.BuildSources(validRuntimeConfig(endpoint), RuntimeDeps{})
			if err != nil {
				t.Fatalf("build sources: %v", err)
			}

			provider, ok := sources[0].(*dockerprovider.Provider)
			if !ok {
				t.Fatalf("expected docker provider, got %T", sources[0])
			}

			if provider.Endpoint() != endpoint {
				t.Fatalf("expected endpoint %q, got %q", endpoint, provider.Endpoint())
			}

			if provider.ClientHost() != endpoint {
				t.Fatalf("expected docker client host %q, got %q", endpoint, provider.ClientHost())
			}
		})
	}
}

type fakeSource struct {
	provider contracts.ProviderRef
}

func (f fakeSource) Provider() contracts.ProviderRef {
	return f.provider
}

func (fakeSource) ListDesired(context.Context) ([]contracts.DesiredRecord, error) {
	return nil, nil
}

type fakeOutput struct {
	provider contracts.ProviderRef
}

func (f fakeOutput) Provider() contracts.ProviderRef {
	return f.provider
}

func (fakeOutput) ListVisible(context.Context) ([]contracts.VisibleRecord, error) {
	return nil, nil
}

func (fakeOutput) Create(context.Context, contracts.DesiredRecord) error {
	return nil
}

func (fakeOutput) Update(context.Context, contracts.VisibleRecord, contracts.DesiredRecord) error {
	return nil
}

func (fakeOutput) Delete(context.Context, contracts.VisibleRecord) error {
	return nil
}

func validRuntimeConfig(endpoint string) config.Config {
	return config.Config{
		Sources: []config.SourceConfig{{
			Type:     "docker",
			Name:     "local-docker",
			Endpoint: endpoint,
		}},
		Outputs: []config.OutputConfig{{
			Type:     "adguard",
			Name:     "primary-adguard",
			URL:      "http://127.0.0.1:3000",
			Username: "admin",
			Password: "secret",
		}},
	}
}
