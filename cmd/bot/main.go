package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"texas-holdem/internal/bot"
	"texas-holdem/internal/config"
	"texas-holdem/internal/store"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.SQLitePath, cfg.InitialBalance, cfg.DailyCheckin)
	if err != nil {
		logger.Error("open store", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	api, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		logger.Error("create telegram bot", "error", err)
		os.Exit(1)
	}
	logger.Info("bot authorized", "username", api.Self.UserName)

	app := bot.New(api, cfg, st, logger)
	if err := app.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("run bot", "error", err)
		os.Exit(1)
	}
}
