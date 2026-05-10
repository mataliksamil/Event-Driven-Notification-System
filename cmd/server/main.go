package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/samil/notification/internal/adapter/batch"
	"github.com/samil/notification/internal/adapter/middleware"
	"github.com/samil/notification/internal/config"
	"github.com/samil/notification/internal/db"
	"github.com/samil/notification/internal/logger"
	"github.com/samil/notification/internal/metrics"
	"github.com/samil/notification/internal/migration"
	"github.com/samil/notification/internal/producer"
	redisSvc "github.com/samil/notification/internal/redis"
	"github.com/samil/notification/internal/service"
	"github.com/samil/notification/internal/storage"
	"github.com/samil/notification/internal/swagger"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger.Init()

	if err := migration.Run(cfg, "./migrations"); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	if err := swagger.Load("./oapi.yaml"); err != nil {
		slog.Error("failed to load openapi spec", "error", err)
		os.Exit(1)
	}

	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		slog.Error("failed to connect to db", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	redisClient, err := redisSvc.NewClient(ctx, cfg)
	if err != nil {
		slog.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	asynqClient := producer.NewClient(cfg)
	defer asynqClient.Close()

	repo := storage.NewPostgresRepository(pool)
	idempotency := redisSvc.NewIdempotencyService(redisClient)
	prod := producer.NewProducer(asynqClient)

	batchSvc := service.NewBatchService(repo, prod)
	batchHandler := batch.NewHandler(batchSvc)
	idempotencyMW := middleware.NewIdempotency(idempotency)
	requestLogger := middleware.NewRequestLogger()

	r := chi.NewRouter()
	r.Use(requestLogger.Handler)
	r.Get("/metrics", metrics.Handler().ServeHTTP)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	r.Get("/swagger/spec.yaml", swagger.SpecHandler())
	r.Get("/swagger", swagger.UIHandler())
	r.Get("/swagger/", swagger.UIHandler())

	r.Route("/api/v1", func(r chi.Router) {
		r.With(idempotencyMW.Handle).Mount("/notifications/batches", batchHandler.Routes())
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.ServerPort),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server")
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}