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
	"time"

	"booker/internal/auth"
	"booker/internal/config"
	"booker/internal/db"
	"booker/internal/logger"
	"booker/internal/middleware"
	"booker/internal/room"
	"booker/internal/schedule"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
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

	roomService := room.NewService(roomRepo)
	scheduleService := schedule.NewService(scheduleRepo, roomRepo)

	authHandler := auth.NewHTTPHandler(cfg.JWT, logger)
	roomHandler := room.NewHandler(roomService, logger)
	scheduleHandler := schedule.NewHandler(scheduleService, logger)

	r := chi.NewRouter()
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Timeout(5 * time.Second))

	r.Get("/_info", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Post("/dummyLogin", authHandler.DummyLogin)

	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(cfg.JWT.Secret))
		roomHandler.Register(r)
		scheduleHandler.Register(r)
	})

	srv := &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      r,
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

	<-ctx.Done()

	logger.Info("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", slog.String("err", err.Error()))
	}
}
