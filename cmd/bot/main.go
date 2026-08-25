package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mos-sport-bot/internal/config"
	"mos-sport-bot/internal/store"
	"mos-sport-bot/internal/telegram"
	"mos-sport-bot/internal/webhook"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "error", err)
		os.Exit(1)
	}

	st, err := store.New(cfg.SubscribersFile)
	if err != nil {
		logger.Error("store init failed", "error", err)
		os.Exit(1)
	}

	tb, err := telegram.New(cfg.TelegramBotToken, st, cfg.BackendStatusURL, cfg.RequestTimeout, logger)
	if err != nil {
		logger.Error("telegram init failed", "error", err)
		os.Exit(1)
	}

	wh := webhook.New(tb, cfg.WebhookSecret, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go tb.Run(ctx)

	server := &http.Server{Addr: cfg.ListenAddr, Handler: wh.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info("http server started", "addr", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server stopped", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	logger.Info("service stopped")
}
