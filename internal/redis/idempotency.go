package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type IdempotencyService struct {
	client *goredis.Client
	ttl    time.Duration
}

func NewIdempotencyService(client *goredis.Client) *IdempotencyService {
	return &IdempotencyService{
		client: client,
		ttl:    24 * time.Hour,
	}
}

func (s *IdempotencyService) CheckAndSet(ctx context.Context, key string) (bool, error) {
	ok, err := s.client.SetNX(ctx, s.redisKey(key), true, s.ttl).Result()
	if err != nil {
		return false, fmt.Errorf("redis SETNX: %w", err)
	}
	return ok, nil
}

func (s *IdempotencyService) Release(ctx context.Context, key string) error {
	err := s.client.Del(ctx, s.redisKey(key)).Err()
	if err != nil {
		return fmt.Errorf("redis DEL: %w", err)
	}
	return nil
}

func (s *IdempotencyService) redisKey(key string) string {
	return fmt.Sprintf("idempotency:%s", key)
}