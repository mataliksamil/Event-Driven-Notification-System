package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/hibiken/asynq"
)

type Config struct {
	DBHost            string
	DBPort            int
	DBUser            string
	DBPassword        string
	DBName            string
	DBSSLMode         string
	RedisHost         string
	RedisPort         int
	ServerPort        string
	WebhookURL        string
	WorkerConcurrency int
}

func Load() (*Config, error) {
	dbPort, err := strconv.Atoi(envOr("DB_PORT", "5432"))
	if err != nil {
		return nil, fmt.Errorf("invalid DB_PORT: %w", err)
	}

	redisPort, err := strconv.Atoi(envOr("REDIS_PORT", "6379"))
	if err != nil {
		return nil, fmt.Errorf("invalid REDIS_PORT: %w", err)
	}

	workerConcurrency, err := strconv.Atoi(envOr("WORKER_CONCURRENCY", "20"))
	if err != nil {
		return nil, fmt.Errorf("invalid WORKER_CONCURRENCY: %w", err)
	}

	return &Config{
		DBHost:     envOr("DB_HOST", "localhost"),
		DBPort:     dbPort,
		DBUser:     envOr("DB_USER", "samil"),
		DBPassword: envOr("DB_PASSWORD", "mysecretpassword"),
		DBName:     envOr("DB_NAME", "myappdb"),
		DBSSLMode:  envOr("DB_SSLMODE", "disable"),
		RedisHost:  envOr("REDIS_HOST", "localhost"),
		RedisPort:  redisPort,
		ServerPort:        envOr("SERVER_PORT", "8080"),
		WebhookURL:        envOr("WEBHOOK_URL", ""),
		WorkerConcurrency: workerConcurrency,
	}, nil
}

func (c *Config) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode,
	)
}

func (c *Config) AsynqRedisOpt() asynq.RedisClientOpt {
	return asynq.RedisClientOpt{
		Addr: fmt.Sprintf("%s:%d", c.RedisHost, c.RedisPort),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}