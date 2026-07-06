package adguardstub

import (
	"context"

	"github.com/aaronflorey/docker-dns-sync/internal/config"
	"github.com/aaronflorey/docker-dns-sync/internal/contracts"
)

type Provider struct {
	ref contracts.ProviderRef
	url string
}

func New(cfg config.OutputConfig) *Provider {
	return &Provider{
		ref: contracts.ProviderRef{
			Type: cfg.Type,
			Name: cfg.Name,
		},
		url: cfg.URL,
	}
}

func (p *Provider) Provider() contracts.ProviderRef {
	return p.ref
}

func (p *Provider) URL() string {
	return p.url
}

func (p *Provider) ListVisible(context.Context) ([]contracts.VisibleRecord, error) {
	return nil, nil
}

func (p *Provider) Create(context.Context, contracts.DesiredRecord) (*contracts.RecordProvenance, error) {
	return nil, nil
}

func (p *Provider) Update(context.Context, contracts.VisibleRecord, contracts.DesiredRecord) (*contracts.RecordProvenance, error) {
	return nil, nil
}

func (p *Provider) Delete(context.Context, contracts.VisibleRecord) error {
	return nil
}
