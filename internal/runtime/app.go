package runtime

import (
	"context"
	"log/slog"

	"github.com/aaronlmathis/docker-dns-sync/internal/config"
)

type App struct {
	cfg config.Config
}

func New(cfg config.Config) *App {
	return &App{cfg: cfg}
}

func (a *App) Run(ctx context.Context) error {
	logger := slog.Default()
	logger.Info("starting docker-dns-sync runtime", "sources", len(a.cfg.Sources), "outputs", len(a.cfg.Outputs), "state_path", a.cfg.State.Path)
	<-ctx.Done()
	logger.Info("runtime cancelled", "reason", ctx.Err())
	return nil
}
