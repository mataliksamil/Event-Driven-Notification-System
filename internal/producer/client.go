package producer

import (
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/samil/notification/internal/config"
)

func NewClient(cfg *config.Config) *asynq.Client {
	return asynq.NewClient(asynq.RedisClientOpt{
		Addr: fmt.Sprintf("%s:%d", cfg.RedisHost, cfg.RedisPort),
	})
}