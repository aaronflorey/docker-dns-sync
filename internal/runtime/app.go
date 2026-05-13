package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/aaronlmathis/docker-dns-sync/internal/config"
	"github.com/aaronlmathis/docker-dns-sync/internal/contracts"
	"github.com/aaronlmathis/docker-dns-sync/internal/state"
)

type App struct {
	cfg      config.Config
	registry *FactoryRegistry
	deps     RuntimeDeps
	store    *state.Store
	sources  []contracts.Source
	outputs  []contracts.Output
}

func New(cfg config.Config) *App {
	return &App{
		cfg:      cfg,
		registry: NewDefaultFactoryRegistry(),
	}
}

func (a *App) Run(ctx context.Context) error {
	if err := config.Validate(a.cfg); err != nil {
		return err
	}

	resolved, err := config.ResolveSecrets(a.cfg, os.LookupEnv)
	if err != nil {
		return err
	}

	deps, err := NewRuntimeDeps(resolved)
	if err != nil {
		return err
	}

	store, err := state.NewStore(resolved.State.Path)
	if err != nil {
		return err
	}

	sources, outputs, err := a.registry.BuildProviders(resolved, deps)
	if err != nil {
		return err
	}

	a.cfg = resolved
	a.deps = deps
	a.store = store
	a.sources = sources
	a.outputs = outputs

	deps.Logger.Info("starting docker-dns-sync runtime", "sources", len(sources), "outputs", len(outputs), "state_path", resolved.State.Path)
	if err := a.startupReconcile(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	deps.Logger.Info("runtime cancelled", "reason", ctx.Err())
	return nil
}

func (a *App) startupReconcile(ctx context.Context) error {
	owned, err := a.store.Load()
	if err != nil {
		return fmt.Errorf("load owned state: %w", err)
	}

	desired := make([]contracts.DesiredRecord, 0)
	for i, source := range a.sources {
		records, err := source.ListDesired(ctx)
		if err != nil {
			return fmt.Errorf("list desired records from source %d (%s/%s): %w", i, source.Provider().Type, source.Provider().Name, err)
		}
		desired = append(desired, records...)
	}

	for i, output := range a.outputs {
		visible, err := output.ListVisible(ctx)
		if err != nil {
			return fmt.Errorf("list visible records from output %d (%s/%s): %w", i, output.Provider().Type, output.Provider().Name, err)
		}

		result, err := ReconcileAndPersist(ctx, a.store, ReconcileInput{
			Output:  output,
			Desired: desired,
			Visible: visible,
			Owned:   owned,
		})
		if err != nil {
			return fmt.Errorf("startup reconcile output %d (%s/%s): %w", i, output.Provider().Type, output.Provider().Name, err)
		}

		owned = result.Next
	}

	return nil
}

func (a *App) Deps() RuntimeDeps {
	return a.deps
}

func (a *App) LogLevel() slog.Level {
	return a.deps.LogLevel
}

func (a *App) StateStore() *state.Store {
	return a.store
}

func (a *App) SourceCount() int {
	return len(a.sources)
}

func (a *App) OutputCount() int {
	return len(a.outputs)
}
