package runtime

import (
	"context"
	"log/slog"

	"github.com/aaronlmathis/docker-dns-sync/internal/config"
)

type App struct {
	cfg      config.Config
	registry *FactoryRegistry
}

func New(cfg config.Config) *App {
	return &App{
		cfg:      cfg,
		registry: NewDefaultFactoryRegistry(),
	}
}

func (a *App) Run(ctx context.Context) error {
	sources, outputs, err := a.registry.BuildProviders(a.cfg)
	if err != nil {
		return err
	}

	logger := slog.Default()
	logger.Info("starting docker-dns-sync runtime", "sources", len(sources), "outputs", len(outputs), "state_path", a.cfg.State.Path)
	<-ctx.Done()
	logger.Info("runtime cancelled", "reason", ctx.Err())
	return nil
}
