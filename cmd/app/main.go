package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"booker/internal/config"
	"booker/internal/db"
	"booker/internal/logger"
	"booker/internal/room"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	ctx, cancel := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT, syscall.SIGTERM,
	)
	defer cancel()

	logger := logger.New(cfg.Log.Level)

	connectCtx, connectCancel := context.WithTimeout(ctx, cfg.DB.ConnectTimeout)
	defer connectCancel()

	logger.Debug("initializing postgres connection pool")
	pg, err := db.New(connectCtx, cfg.DB)
	if err != nil {
		logger.Error("failed to connect to postgres", slog.String("err", err.Error()))
		os.Exit(1)
	}
	defer pg.Close()

	roomRepo := room.NewRepository(pg)
}
