package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string
	ServerPort string
}

func Load() (*Config, error) {
	port, err := strconv.Atoi(envOr("DB_PORT", "5432"))
	if err != nil {
		return nil, fmt.Errorf("invalid DB_PORT: %w", err)
	}

	return &Config{
		DBHost:     envOr("DB_HOST", "localhost"),
		DBPort:     port,
		DBUser:     envOr("DB_USER", "samil"),
		DBPassword: envOr("DB_PASSWORD", "mysecretpassword"),
		DBName:     envOr("DB_NAME", "myappdb"),
		DBSSLMode:  envOr("DB_SSLMODE", "disable"),
		ServerPort: envOr("SERVER_PORT", "8080"),
	}, nil
}

func (c *Config) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode,
	)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}