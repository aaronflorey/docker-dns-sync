package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/aaronlmathis/docker-dns-sync/internal/config"
	"github.com/aaronlmathis/docker-dns-sync/internal/runtime"
)

type appRunner interface {
	Run(context.Context) error
}

var newApp = func(cfg config.Config) appRunner {
	return runtime.New(cfg)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runWithContext(ctx, args)
}

func runWithContext(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("docker-dns-sync", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "Path to the TOML config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *configPath == "" {
		return errors.New("-config is required")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	app := newApp(cfg)
	return app.Run(ctx)
}
