package producer

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/samil/notification/internal/adapter/middleware"
	"github.com/samil/notification/internal/domain"
	"github.com/samil/notification/internal/logger"
	"github.com/samil/notification/internal/worker"
)

type Producer struct {
	client *asynq.Client
}

func NewProducer(client *asynq.Client) *Producer {
	return &Producer{client: client}
}

func (p *Producer) Enqueue(ctx context.Context, notification *domain.Notification) error {
	correlationID := middleware.CorrelationIDFromContext(ctx)

	task, err := channelToTask(notification.Channel, notification.ID, correlationID)
	if err != nil {
		return err
	}

	queueName := priorityToQueue(notification.Priority)

	log := logger.FromContext(ctx).With(
		"component", "producer",
		"notification_id", notification.ID,
		"channel", notification.Channel,
		"priority", notification.Priority,
		"queue", queueName,
		"task_type", task.Type(),
	)

	if _, err = p.client.Enqueue(task, asynq.Queue(queueName)); err != nil {
		log.Error("failed to enqueue task", "error", err)
		return fmt.Errorf("enqueue task: %w", err)
	}

	log.Info("task enqueued")
	return nil
}

func priorityToQueue(p domain.Priority) string {
	switch p {
	case domain.PriorityHigh:
		return "critical"
	case domain.PriorityLow:
		return "low"
	default:
		return "default"
	}
}

func channelToTask(ch domain.Channel, id uuid.UUID, correlationID string) (*asynq.Task, error) {
	switch ch {
	case domain.ChannelSMS:
		return worker.NewTaskSMS(id, correlationID)
	case domain.ChannelEmail:
		return worker.NewTaskEmail(id, correlationID)
	case domain.ChannelPush:
		return worker.NewTaskPush(id, correlationID)
	default:
		return nil, fmt.Errorf("unknown channel: %s", ch)
	}
}