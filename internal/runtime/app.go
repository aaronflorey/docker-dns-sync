package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/aaronflorey/docker-dns-sync/internal/config"
	"github.com/aaronflorey/docker-dns-sync/internal/contracts"
	"github.com/aaronflorey/docker-dns-sync/internal/state"
)

var errSourceWatchClosed = errors.New("source watch closed")

type App struct {
	cfg      config.Config
	registry *FactoryRegistry
	newDeps  func(config.Config) (RuntimeDeps, error)
	deps     RuntimeDeps
	store    *state.Store
	sources  []contracts.Source
	outputs  []contracts.Output
}

func New(cfg config.Config) *App {
	return &App{
		cfg:      cfg,
		registry: NewDefaultFactoryRegistry(),
		newDeps:  NewRuntimeDeps,
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

	deps, err := a.newDeps(resolved)
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
	a.outputs = wrapOutputs(outputs, deps)

	deps.Logger.Info("starting docker-dns-sync runtime", "sources", len(sources), "outputs", len(outputs), "state_path", resolved.State.Path, "log_level", resolved.Logging.Level)
	if err := a.reconcile(ctx, "startup"); err != nil {
		if errors.Is(err, context.Canceled) {
			deps.Logger.Info("runtime cancelled", "reason", ctx.Err())
			return nil
		}
		return err
	}
	if err := a.runSteadyState(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			deps.Logger.Info("runtime cancelled", "reason", ctx.Err())
			return nil
		}
		return err
	}
	return nil
}

func (a *App) reconcile(ctx context.Context, reason string) error {
	a.deps.Logger.Info("starting reconcile", "reason", reason)
	startedAt := time.Now()
	delay := a.deps.Retry.InitialInterval

	for attempt := 1; ; attempt++ {
		err := a.reconcileOnce(ctx)
		if err == nil {
			a.deps.Logger.Info("reconcile completed", "reason", reason)
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		var sourceErr sourceListError
		if errors.As(err, &sourceErr) && isTemporaryError(err) {
			if time.Since(startedAt)+delay > a.deps.Retry.MaxElapsedTime {
				return err
			}

			a.deps.Logger.Warn("retrying full reconcile after temporary source read failure", "reason", reason, "source", providerKey(sourceErr.provider), "attempt", attempt, "next_delay", delay, "error", err)
			if err := sleepContext(ctx, delay); err != nil {
				return err
			}

			delay = nextBackoffDelay(delay, a.deps.Retry.MaxInterval)
			continue
		}

		var outputErr outputListError
		if errors.As(err, &outputErr) && isTemporaryError(err) {
			if time.Since(startedAt)+delay > a.deps.Retry.MaxElapsedTime {
				return err
			}

			a.deps.Logger.Warn("retrying full reconcile after temporary output read failure", "reason", reason, "output", providerKey(outputErr.provider), "attempt", attempt, "next_delay", delay, "error", err)
			if err := sleepContext(ctx, delay); err != nil {
				return err
			}

			delay = nextBackoffDelay(delay, a.deps.Retry.MaxInterval)
			continue
		}

		var mutationErr outputMutationError
		if !errors.As(err, &mutationErr) || !isTemporaryError(err) {
			return err
		}
		if time.Since(startedAt)+delay > a.deps.Retry.MaxElapsedTime {
			return err
		}

		a.deps.Logger.Warn("retrying full reconcile after temporary output write failure", "reason", reason, "output", providerKey(mutationErr.provider), "attempt", attempt, "next_delay", delay, "error", err)
		if err := sleepContext(ctx, delay); err != nil {
			return err
		}

		delay = nextBackoffDelay(delay, a.deps.Retry.MaxInterval)
	}
}

func (a *App) reconcileOnce(ctx context.Context) error {
	owned, err := a.store.Load()
	if err != nil {
		return fmt.Errorf("load owned state: %w", err)
	}
	logDebug(ctx, a.deps.Logger, "loaded owned state snapshot", "records", len(owned.ManagedRecords))

	desired := make([]contracts.DesiredRecord, 0)
	for i, source := range a.sources {
		records, err := source.ListDesired(ctx)
		if err != nil {
			return sourceListError{sourceIndex: i, provider: source.Provider(), err: err}
		}
		logDebug(ctx, a.deps.Logger, "listed desired records from source", "source", providerKey(source.Provider()), "records", len(records))
		for _, record := range records {
			logTrace(ctx, a.deps.Logger, "desired record discovered", "source", providerKey(source.Provider()), "source_id", record.Source.ID, "display_name", record.Source.DisplayName, "hostname", record.Hostname, "answer", record.Answer)
		}
		desired = append(desired, records...)
	}
	logDebug(ctx, a.deps.Logger, "collected desired records across sources", "records", len(desired))

	for i, output := range a.outputs {
		visible, err := output.ListVisible(ctx)
		if err != nil {
			return outputListError{outputIndex: i, provider: output.Provider(), err: err}
		}
		logDebug(ctx, a.deps.Logger, "listed visible records from output", "output", providerKey(output.Provider()), "records", len(visible))
		for _, record := range visible {
			logTrace(ctx, a.deps.Logger, "visible record discovered", "output", providerKey(output.Provider()), "hostname", record.Hostname, "answer", record.Answer)
		}

		result, err := ReconcileAndPersist(ctx, a.store, ReconcileInput{
			Output:  output,
			Desired: desired,
			Visible: visible,
			Owned:   owned,
			Logger:  a.deps.Logger,
		})
		if err != nil {
			return fmt.Errorf("reconcile output %d (%s/%s): %w", i, output.Provider().Type, output.Provider().Name, err)
		}

		owned = result.Next
	}
	return nil
}

type sourceWatchEvent struct {
	sourceIndex int
	err         error
	hint        bool
	reconnect   bool
}

type sourceListError struct {
	sourceIndex int
	provider    contracts.ProviderRef
	err         error
}

type outputListError struct {
	outputIndex int
	provider    contracts.ProviderRef
	err         error
}

type temporaryError interface {
	Temporary() bool
}

func (e sourceListError) Error() string {
	return fmt.Sprintf("list desired records from source %d (%s/%s): %v", e.sourceIndex, e.provider.Type, e.provider.Name, e.err)
}

func (e sourceListError) Unwrap() error {
	return e.err
}

func (e outputListError) Error() string {
	return fmt.Sprintf("list visible records from output %d (%s/%s): %v", e.outputIndex, e.provider.Type, e.provider.Name, e.err)
}

func (e outputListError) Unwrap() error {
	return e.err
}

func isTemporaryError(err error) bool {
	var temporaryErr temporaryError
	return errors.As(err, &temporaryErr) && temporaryErr.Temporary()
}

func (a *App) runSteadyState(ctx context.Context) error {
	watchers := make(map[int]contracts.WatchableSource)
	for i, source := range a.sources {
		watchable, ok := source.(contracts.WatchableSource)
		if ok {
			watchers[i] = watchable
		}
	}

	if len(watchers) == 0 {
		<-ctx.Done()
		return ctx.Err()
	}

	events := make(chan sourceWatchEvent, len(watchers))
	reconnectDelays := make(map[int]time.Duration, len(watchers))
	reconnectPending := make(map[int]bool, len(watchers))
	for i, watchable := range watchers {
		reconnectDelays[i] = a.deps.Retry.InitialInterval
		a.startSourceWatch(ctx, i, watchable, events)
	}
	if err := a.reconcile(ctx, "watch_startup_handoff"); err != nil {
		return err
	}
	for i := range reconnectDelays {
		reconnectDelays[i] = a.deps.Retry.InitialInterval
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-events:
			if event.hint {
				logDebug(ctx, a.deps.Logger, "source watch emitted reconcile hint", "source", providerKey(a.sources[event.sourceIndex].Provider()))
				reconnectDelays[event.sourceIndex] = a.deps.Retry.InitialInterval
				if err := a.reconcile(ctx, "watch_hint"); err != nil {
					return err
				}
				continue
			}

			if event.reconnect {
				reconnectPending[event.sourceIndex] = false
				logDebug(ctx, a.deps.Logger, "restarting source watch after backoff", "source", providerKey(a.sources[event.sourceIndex].Provider()))
				a.startSourceWatch(ctx, event.sourceIndex, watchers[event.sourceIndex], events)
				if err := a.reconcile(ctx, "watch_reconnect_repair"); err != nil {
					return err
				}
				reconnectDelays[event.sourceIndex] = a.deps.Retry.InitialInterval
				continue
			}

			provider := a.sources[event.sourceIndex].Provider()
			if reconnectPending[event.sourceIndex] {
				continue
			}
			delay := reconnectDelays[event.sourceIndex]
			a.deps.Logger.Warn("source watch disconnected", "source", providerKey(provider), "reconnect_delay", delay, "error", event.err)
			reconnectPending[event.sourceIndex] = true
			reconnectDelays[event.sourceIndex] = nextBackoffDelay(delay, a.deps.Retry.MaxInterval)
			a.scheduleSourceReconnect(ctx, event.sourceIndex, delay, events)
		}
	}
}

func (a *App) scheduleSourceReconnect(ctx context.Context, sourceIndex int, delay time.Duration, events chan<- sourceWatchEvent) {
	go func() {
		if err := sleepContext(ctx, delay); err != nil {
			return
		}

		select {
		case events <- sourceWatchEvent{sourceIndex: sourceIndex, reconnect: true}:
		case <-ctx.Done():
		}
	}()
}

func nextBackoffDelay(delay, maxInterval time.Duration) time.Duration {
	if delay <= 0 {
		return 0
	}
	if maxInterval > 0 && delay > maxInterval {
		return maxInterval
	}
	next := delay * 2
	if maxInterval > 0 && next > maxInterval {
		return maxInterval
	}
	return next
}

func (a *App) startSourceWatch(ctx context.Context, sourceIndex int, source contracts.WatchableSource, events chan<- sourceWatchEvent) {
	provider := a.sources[sourceIndex].Provider()
	a.deps.Logger.Info("starting source watch", "source", providerKey(provider))
	stream := source.Watch(ctx)

	go func() {
		for stream.Hints != nil || stream.Err != nil {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-stream.Hints:
				if !ok {
					stream.Hints = nil
					continue
				}
				logTrace(ctx, a.deps.Logger, "source watch hint received", "source", providerKey(provider))
				select {
				case events <- sourceWatchEvent{sourceIndex: sourceIndex, hint: true}:
				case <-ctx.Done():
				}
			case err, ok := <-stream.Err:
				if !ok {
					stream.Err = nil
					continue
				}
				if err == nil || errors.Is(err, context.Canceled) {
					return
				}
				select {
				case events <- sourceWatchEvent{sourceIndex: sourceIndex, err: err}:
				case <-ctx.Done():
				}
				return
			}
		}
		if ctx.Err() != nil {
			return
		}
		select {
		case events <- sourceWatchEvent{sourceIndex: sourceIndex, err: errSourceWatchClosed}:
		case <-ctx.Done():
		}
	}()
}

type loggingOutput struct {
	base   contracts.Output
	logger *slog.Logger
}

func wrapOutputs(outputs []contracts.Output, deps RuntimeDeps) []contracts.Output {
	wrapped := make([]contracts.Output, 0, len(outputs))
	for _, output := range outputs {
		wrapped = append(wrapped, loggingOutput{base: output, logger: deps.Logger})
	}
	return wrapped
}

func (o loggingOutput) Provider() contracts.ProviderRef {
	return o.base.Provider()
}

func (o loggingOutput) ListVisible(ctx context.Context) ([]contracts.VisibleRecord, error) {
	return o.base.ListVisible(ctx)
}

func (o loggingOutput) Create(ctx context.Context, desired contracts.DesiredRecord) error {
	err := o.base.Create(ctx, desired)
	if err == nil {
		o.logger.Info("output mutation applied", "operation", "create", "provider", providerKey(o.Provider()), "hostname", desired.Hostname)
	}
	return err
}

func (o loggingOutput) Update(ctx context.Context, visible contracts.VisibleRecord, desired contracts.DesiredRecord) error {
	err := o.base.Update(ctx, visible, desired)
	if err == nil {
		o.logger.Info("output mutation applied", "operation", "update", "provider", providerKey(o.Provider()), "hostname", desired.Hostname)
	}
	return err
}

func (o loggingOutput) Delete(ctx context.Context, visible contracts.VisibleRecord) error {
	err := o.base.Delete(ctx, visible)
	if err == nil {
		o.logger.Info("output mutation applied", "operation", "delete", "provider", providerKey(o.Provider()), "hostname", visible.Hostname)
	}
	return err
}

func providerKey(provider contracts.ProviderRef) string {
	return provider.Type + "/" + provider.Name
}

func logDebug(ctx context.Context, logger *slog.Logger, msg string, args ...any) {
	if logger == nil || !logger.Enabled(ctx, slog.LevelDebug) {
		return
	}
	logger.DebugContext(ctx, msg, args...)
}

func logTrace(ctx context.Context, logger *slog.Logger, msg string, args ...any) {
	if logger == nil || !logger.Enabled(ctx, LevelTrace) {
		return
	}
	logger.Log(ctx, LevelTrace, msg, args...)
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
