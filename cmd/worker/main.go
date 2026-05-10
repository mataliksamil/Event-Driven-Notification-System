package main

import (
	"context"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/samil/notification/internal/config"
	"github.com/samil/notification/internal/db"
	"github.com/samil/notification/internal/delivery"
	"github.com/samil/notification/internal/logger"
	"github.com/samil/notification/internal/migration"
	"github.com/samil/notification/internal/storage"
	"github.com/samil/notification/internal/worker"
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

	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		slog.Error("failed to connect to db", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	repo := storage.NewPostgresRepository(pool)
	webhookClient := delivery.NewWebhookClient(cfg.WebhookURL)
	processor := worker.NewNotificationProcessor(repo, webhookClient)

	srv := asynq.NewServer(cfg.AsynqRedisOpt(), asynq.Config{
		Concurrency: cfg.WorkerConcurrency,
		Queues: map[string]int{
			"critical": 10,
			"default":  5,
			"low":      1,
		},
		RetryDelayFunc: exponentialBackoffWithJitter,
	})

	mux := asynq.NewServeMux()
	mux.HandleFunc(worker.TaskDeliverySMS, processor.ProcessTask)
	mux.HandleFunc(worker.TaskDeliveryEmail, processor.ProcessTask)
	mux.HandleFunc(worker.TaskDeliveryPush, processor.ProcessTask)

	slog.Info("starting worker server", "concurrency", cfg.WorkerConcurrency)

	go func() {
		if err := srv.Run(mux); err != nil {
			slog.Error("worker server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down worker")
	srv.Shutdown()
	slog.Info("worker stopped")
}

func exponentialBackoffWithJitter(n int, _ error, _ *asynq.Task) time.Duration {
	base := time.Duration(n+1) * 10 * time.Second
	if base > time.Minute {
		base = time.Minute
	}
	jitter := time.Duration(rand.Intn(2000)) * time.Millisecond
	return base + jitter
}