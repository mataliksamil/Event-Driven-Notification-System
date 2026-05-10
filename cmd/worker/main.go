package main

import (
	"context"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/samil/notification/internal/config"
	"github.com/samil/notification/internal/db"
	"github.com/samil/notification/internal/delivery"
	"github.com/samil/notification/internal/migration"
	"github.com/samil/notification/internal/storage"
	"github.com/samil/notification/internal/worker"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if err := migration.Run(cfg, "./migrations"); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		log.Fatalf("connect db: %v", err)
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

	go func() {
		log.Println("starting worker server...")
		if err := srv.Run(mux); err != nil {
			log.Fatalf("worker server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down worker...")
	srv.Shutdown()
	log.Println("worker stopped")
}

func exponentialBackoffWithJitter(n int, _ error, _ *asynq.Task) time.Duration {
	base := time.Duration(n+1) * 10 * time.Second
	if base > time.Minute {
		base = time.Minute
	}
	jitter := time.Duration(rand.Intn(2000)) * time.Millisecond
	return base + jitter
}
