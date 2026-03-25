package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"booker/internal/auth"
	"booker/internal/booking"
	"booker/internal/conference"
	"booker/internal/config"
	"booker/internal/db"
	"booker/internal/logger"
	"booker/internal/room"
	"booker/internal/schedule"
	"booker/internal/slot"
	"booker/internal/user"
	"booker/internal/worker"
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
	scheduleRepo := schedule.NewRepository(pg)
	slotRepo := slot.NewRepository(pg)
	bookingRepo := booking.NewRepository(pg)
	userRepo := user.NewRepository(pg)
	conferenceService := conference.NewMockService(cfg.Conference)
	slotBuilder := slot.NewBuilder()

	roomService := room.NewService(roomRepo)
	scheduleService := schedule.NewService(scheduleRepo, roomRepo, slotRepo, slotBuilder, cfg.Worker.SlotGenerationDays, logger)
	slotService := slot.NewService(slotRepo, roomRepo)
	bookingService := booking.NewService(bookingRepo, slotRepo, conferenceService)

	authHandler := auth.NewHTTPHandler(cfg.JWT, userRepo, userRepo, logger)
	roomHandler := room.NewHandler(roomService, logger)
	scheduleHandler := schedule.NewHandler(scheduleService, logger)
	slotHandler := slot.NewHandler(slotService, logger)
	bookingHandler := booking.NewHandler(bookingService, logger)

	srv := &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      newRouter(cfg.JWT.Secret, authHandler, roomHandler, scheduleHandler, slotHandler, bookingHandler),
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	go func() {
		logger.Info("server listening", slog.String("addr", cfg.HTTP.Addr))
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", slog.String("err", err.Error()))
			os.Exit(1)
		}
	}()

	go func() {
		worker := worker.NewSlotWorker(
			scheduleRepo,
			slotRepo,
			slotBuilder,
			cfg.Worker.SlotGenerationDays,
			cfg.Worker.Interval,
			logger,
		)
		worker.Run(ctx)
	}()

	<-ctx.Done()

	logger.Info("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", slog.String("err", err.Error()))
	}
}
