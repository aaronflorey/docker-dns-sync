package docker

import (
	"context"
	"fmt"

	"github.com/aaronlmathis/docker-dns-sync/internal/config"
	"github.com/aaronlmathis/docker-dns-sync/internal/contracts"
	mobyclient "github.com/moby/moby/client"
)

type apiClient interface {
	Close() error
	DaemonHost() string
}

type Provider struct {
	ref      contracts.ProviderRef
	endpoint string
	client   apiClient
}

func New(cfg config.SourceConfig) (*Provider, error) {
	cli, err := mobyclient.NewClientWithOpts(
		mobyclient.WithHost(cfg.Endpoint),
		mobyclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}

	return &Provider{
		ref: contracts.ProviderRef{
			Type: cfg.Type,
			Name: cfg.Name,
		},
		endpoint: cfg.Endpoint,
		client:   cli,
	}, nil
}

func (p *Provider) Provider() contracts.ProviderRef {
	return p.ref
}

func (p *Provider) Endpoint() string {
	return p.endpoint
}

func (p *Provider) ClientHost() string {
	if p.client == nil {
		return ""
	}

	return p.client.DaemonHost()
}

func (p *Provider) ListDesired(context.Context) ([]contracts.DesiredRecord, error) {
	return nil, nil
}
