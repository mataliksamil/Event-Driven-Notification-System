package main

import (
	"context"
	"errors"
	"fmt"
	"log"
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
		log.Fatalf("load config: %v", err)
	}

	if err := migration.Run(cfg, "./migrations"); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	if err := swagger.Load("./oapi.yaml"); err != nil {
		log.Fatalf("load openapi spec: %v", err)
	}

	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	redisClient, err := redisSvc.NewClient(ctx, cfg)
	if err != nil {
		log.Fatalf("connect redis: %v", err)
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

	r := chi.NewRouter()
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
		log.Printf("server listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down server...")
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server shutdown error: %v", err)
	}
	log.Println("server stopped")
}