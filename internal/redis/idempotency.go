package redis

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type IdempotencyStatus int

const (
	StatusNew IdempotencyStatus = iota
	StatusProcessing
	StatusCompleted
)

type IdempotencyResult struct {
	Status         IdempotencyStatus
	CachedResponse []byte
}

type IdempotencyService struct {
	client        *goredis.Client
	processingTTL time.Duration
	completedTTL  time.Duration
}

func NewIdempotencyService(client *goredis.Client) *IdempotencyService {
	return &IdempotencyService{
		client:        client,
		processingTTL: 60 * time.Second,
		completedTTL:  24 * time.Hour,
	}
}

func (s *IdempotencyService) Acquire(ctx context.Context, key string) (*IdempotencyResult, error) {
	log := slog.With("component", "idempotency", "key", key)

	val, err := s.client.Get(ctx, s.redisKey(key)).Result()
	if err != nil && err != goredis.Nil {
		log.Error("redis GET failed", "error", err)
		return nil, fmt.Errorf("redis GET: %w", err)
	}

	if err == nil {
		if val == "processing" {
			log.Info("idempotency key found in processing state")
			return &IdempotencyResult{Status: StatusProcessing}, nil
		}

		if len(val) > len(completedPrefix) && val[:len(completedPrefix)] == completedPrefix {
			cached := []byte(val[len(completedPrefix):])
			log.Info("idempotency key found in completed state")
			return &IdempotencyResult{Status: StatusCompleted, CachedResponse: cached}, nil
		}

		return &IdempotencyResult{Status: StatusProcessing}, nil
	}

	ok, err := s.client.SetNX(ctx, s.redisKey(key), "processing", s.processingTTL).Result()
	if err != nil {
		log.Error("redis SETNX failed", "error", err)
		return nil, fmt.Errorf("redis SETNX: %w", err)
	}
	if !ok {
		log.Info("idempotency key acquired by another process")
		return &IdempotencyResult{Status: StatusProcessing}, nil
	}

	log.Info("idempotency key acquired")
	return &IdempotencyResult{Status: StatusNew}, nil
}

func (s *IdempotencyService) Complete(ctx context.Context, key string, responseBody []byte) error {
	log := slog.With("component", "idempotency", "key", key)

	val := completedPrefix + string(responseBody)
	err := s.client.Set(ctx, s.redisKey(key), val, s.completedTTL).Err()
	if err != nil {
		log.Error("redis SET completed failed", "error", err)
		return fmt.Errorf("redis SET completed: %w", err)
	}

	log.Info("idempotency key marked as completed")
	return nil
}

func (s *IdempotencyService) Release(ctx context.Context, key string) error {
	log := slog.With("component", "idempotency", "key", key)

	err := s.client.Del(ctx, s.redisKey(key)).Err()
	if err != nil {
		log.Error("redis DEL failed", "error", err)
		return fmt.Errorf("redis DEL: %w", err)
	}

	log.Info("idempotency key released")
	return nil
}

func (s *IdempotencyService) redisKey(key string) string {
	return fmt.Sprintf("idempotency:%s", key)
}

const completedPrefix = "completed:"