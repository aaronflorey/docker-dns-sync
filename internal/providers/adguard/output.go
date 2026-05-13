package adguard

import (
	"context"
	"errors"

	"github.com/aaronlmathis/docker-dns-sync/internal/config"
	"github.com/aaronlmathis/docker-dns-sync/internal/contracts"
)

type Provider struct {
	ref      contracts.ProviderRef
	baseURL  string
	username string
	password string
}

func New(cfg config.OutputConfig) *Provider {
	return &Provider{
		ref: contracts.ProviderRef{
			Type: cfg.Type,
			Name: cfg.Name,
		},
		baseURL:  cfg.URL,
		username: cfg.Username,
		password: cfg.Password,
	}
}

func (p *Provider) Provider() contracts.ProviderRef {
	return p.ref
}

func (p *Provider) ListVisible(context.Context) ([]contracts.VisibleRecord, error) {
	return nil, errors.New("not implemented")
}

func (p *Provider) Create(context.Context, contracts.DesiredRecord) error {
	return errors.New("not implemented")
}

func (p *Provider) Update(context.Context, contracts.VisibleRecord, contracts.DesiredRecord) error {
	return errors.New("not implemented")
}

func (p *Provider) Delete(context.Context, contracts.VisibleRecord) error {
	return errors.New("not implemented")
}
