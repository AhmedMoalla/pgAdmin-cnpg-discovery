package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/AhmedMoalla/pgadmin-cnpg-discovery/internal/config"
	"github.com/AhmedMoalla/pgadmin-cnpg-discovery/internal/discovery"
	"github.com/AhmedMoalla/pgadmin-cnpg-discovery/internal/reconciler"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: getLogLevel(),
	})))

	slog.Info("starting pgadmin-cnpg-discovery")

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	disc, err := discovery.New(cfg.Namespace)
	if err != nil {
		slog.Error("failed to create discoverer", "error", err)
		os.Exit(1)
	}

	rec := reconciler.New(cfg, disc)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	if err := rec.Run(ctx); err != nil && ctx.Err() == nil {
		slog.Error("reconciler exited with error", "error", err)
		os.Exit(1)
	}

	slog.Info("shutdown complete")
}

func getLogLevel() slog.Level {
	switch os.Getenv("LOG_LEVEL") {
	case "debug", "DEBUG":
		return slog.LevelDebug
	case "warn", "WARN":
		return slog.LevelWarn
	case "error", "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
