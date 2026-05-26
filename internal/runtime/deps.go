package runtime

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/aaronflorey/docker-dns-sync/internal/config"
)

const LevelTrace slog.Level = slog.LevelDebug - 4

type RetryPolicy struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	MaxElapsedTime  time.Duration
}

type RuntimeDeps struct {
	Logger   *slog.Logger
	LogLevel slog.Level
	Retry    RetryPolicy
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
		Logger:   slog.New(handler),
		LogLevel: level,
		Retry:    retry,
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

func retryWithBackoff(ctx context.Context, policy RetryPolicy, operation func(attempt int) error, onRetry func(attempt int, delay time.Duration, err error)) error {
	startedAt := time.Now()
	delay := policy.InitialInterval

	for attempt := 1; ; attempt++ {
		err := operation(attempt)
		if err == nil {
			return nil
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
