// Command pcb runs the ProxyConnectorBot: a Telegram bot managing HWID-bound
// VPN subscriptions plus an HTTP endpoint serving them on our own domain.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/infybtw/ProxyConnectorBot/internal/botapp"
	"github.com/infybtw/ProxyConnectorBot/internal/config"
	"github.com/infybtw/ProxyConnectorBot/internal/origin"
	"github.com/infybtw/ProxyConnectorBot/internal/store"
	"github.com/infybtw/ProxyConnectorBot/internal/web"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	// Load .env if present (dev convenience; compose passes real env vars).
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config error", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("db: connect failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	st := store.New(pool)
	if err := st.Migrate(ctx); err != nil {
		slog.Error("db: migrate failed", "err", err)
		os.Exit(1)
	}

	oc := origin.NewClient(cfg.OriginTimeout, cfg.OriginMaxBody, origin.Device{
		OS:        cfg.HWIDDeviceOS,
		OSVersion: cfg.HWIDVerOS,
		Model:     cfg.HWIDDeviceModel,
		UserAgent: cfg.OriginUserAgent,
	})

	server := web.NewServer(st, oc)
	httpErr := make(chan error, 1)
	go func() {
		slog.Info("http: listening", "addr", cfg.HTTPAddr)
		if err := server.Listen(cfg.HTTPAddr); err != nil && !errors.Is(err, context.Canceled) {
			httpErr <- err
		}
	}()

	bot := botapp.New(st, oc, cfg)
	botErr := make(chan error, 1)
	go func() {
		slog.Info("bot: polling started")
		botErr <- bot.Run(ctx)
	}()

	select {
	case err := <-httpErr:
		slog.Error("http: server failed", "err", err)
		stop()
	case err := <-botErr:
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("bot: polling failed", "err", err)
		}
		stop()
	case <-ctx.Done():
		slog.Info("shutdown requested")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		if err := server.Shutdown(); err != nil {
			slog.Error("http: shutdown failed", "err", err)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-shutdownCtx.Done():
		slog.Warn("http: shutdown timed out")
	}
	pool.Close()
	slog.Info("bye")
}
