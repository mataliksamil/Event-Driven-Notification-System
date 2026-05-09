package producer

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/samil/notification/internal/domain"
	"github.com/samil/notification/internal/worker"
)

type Producer struct {
	client *asynq.Client
}

func NewProducer(client *asynq.Client) *Producer {
	return &Producer{client: client}
}

func (p *Producer) Enqueue(ctx context.Context, notification *domain.Notification) error {
	task, err := channelToTask(notification.Channel, notification.ID)
	if err != nil {
		return err
	}

	queueName := priorityToQueue(notification.Priority)

	if _, err = p.client.Enqueue(task, asynq.Queue(queueName)); err != nil {
		return fmt.Errorf("enqueue task: %w", err)
	}

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

func channelToTask(ch domain.Channel, id uuid.UUID) (*asynq.Task, error) {
	switch ch {
	case domain.ChannelSMS:
		return worker.NewTaskSMS(id)
	case domain.ChannelEmail:
		return worker.NewTaskEmail(id)
	case domain.ChannelPush:
		return worker.NewTaskPush(id)
	default:
		return nil, fmt.Errorf("unknown channel: %s", ch)
	}
}