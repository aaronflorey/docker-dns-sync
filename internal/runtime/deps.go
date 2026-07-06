package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/aaronflorey/docker-dns-sync/internal/config"
)

const (
	LevelTrace               slog.Level = slog.LevelDebug - 4
	defaultWatchHintDebounce            = 500 * time.Millisecond
	defaultOperationTimeout             = 10 * time.Second
)

type RetryPolicy struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	MaxElapsedTime  time.Duration
}

type RuntimeDeps struct {
	Logger            *slog.Logger
	LogLevel          slog.Level
	OperationTimeout  time.Duration
	Retry             RetryPolicy
	WatchHintDebounce time.Duration
}

func NewRuntimeDeps(cfg config.Config) (RuntimeDeps, error) {
	level, err := parseLogLevel(cfg.Logging.Level)
	if err != nil {
		return RuntimeDeps{}, err
	}

	retry, err := parseRetryPolicy(cfg.Retry)
	if err != nil {
		return RuntimeDeps{}, err
	}

	operationTimeout, err := parseOperationTimeout(cfg.Runtime)
	if err != nil {
		return RuntimeDeps{}, err
	}

	handlerOptions := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(cfg.Logging.Format)) {
	case "json":
		handler = slog.NewJSONHandler(os.Stderr, handlerOptions)
	case "text":
		handler = slog.NewTextHandler(os.Stderr, handlerOptions)
	default:
		handler = slog.NewTextHandler(io.Discard, handlerOptions)
	}

	return RuntimeDeps{
		Logger:            slog.New(handler),
		LogLevel:          level,
		OperationTimeout:  operationTimeout,
		Retry:             retry,
		WatchHintDebounce: defaultWatchHintDebounce,
	}, nil
}

func parseLogLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "trace":
		return LevelTrace, nil
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("logging.level must be one of trace, debug, info, warn, error")
	}
}

func parseRetryPolicy(cfg config.RetryConfig) (RetryPolicy, error) {
	initial, err := time.ParseDuration(cfg.InitialInterval)
	if err != nil {
		return RetryPolicy{}, fmt.Errorf("parse retry.initial_interval: %w", err)
	}

	maxInterval, err := time.ParseDuration(cfg.MaxInterval)
	if err != nil {
		return RetryPolicy{}, fmt.Errorf("parse retry.max_interval: %w", err)
	}

	maxElapsed, err := time.ParseDuration(cfg.MaxElapsedTime)
	if err != nil {
		return RetryPolicy{}, fmt.Errorf("parse retry.max_elapsed_time: %w", err)
	}

	return RetryPolicy{
		InitialInterval: initial,
		MaxInterval:     maxInterval,
		MaxElapsedTime:  maxElapsed,
	}, nil
}

func parseOperationTimeout(cfg config.RuntimeConfig) (time.Duration, error) {
	value := strings.TrimSpace(cfg.OperationTimeout)
	if value == "" {
		return defaultOperationTimeout, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse runtime.operation_timeout: %w", err)
	}

	if duration <= 0 {
		return 0, fmt.Errorf("runtime.operation_timeout must be greater than zero")
	}

	return duration, nil
}

func retryWithBackoff(ctx context.Context, policy RetryPolicy, operation func(attempt int) error, onRetry func(attempt int, delay time.Duration, err error)) error {
	startedAt := time.Now()
	delay := policy.InitialInterval

	for attempt := 1; ; attempt++ {
		err := operation(attempt)
		if err == nil {
			return nil
		}
		if isContextCancellationError(err) {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if time.Since(startedAt)+delay > policy.MaxElapsedTime {
			return err
		}

		if onRetry != nil {
			onRetry(attempt, delay, err)
		}

		if err := sleepContext(ctx, delay); err != nil {
			return err
		}

		delay *= 2
		if delay > policy.MaxInterval {
			delay = policy.MaxInterval
		}
	}
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func withOperationTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

func isContextCancellationError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func isTemporaryError(err error) bool {
	if isContextCancellationError(err) {
		return false
	}

	var temporaryErr temporaryError
	return errors.As(err, &temporaryErr) && temporaryErr.Temporary()
}
