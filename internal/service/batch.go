package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/samil/notification/internal/domain"
)

type BatchService struct {
	repo domain.NotificationRepository
}

func NewBatchService(repo domain.NotificationRepository) *BatchService {
	return &BatchService{repo: repo}
}

type NotificationInput struct {
	Recipient string
	Channel   string
	Content   string
	Priority  string
}

type BatchResult struct {
	BatchID    string
	Status     string
	TotalCount int
	AcceptedAt time.Time
}

func (s *BatchService) CreateBatch(ctx context.Context, idempotencyKey uuid.UUID, inputs []NotificationInput) (*BatchResult, error) {
	batchID := uuid.New()
	now := time.Now().UTC()

	notifications, err := buildNotifications(batchID, now, inputs)
	if err != nil {
		return nil, err
	}

	batch := &domain.Batch{
		ID:             batchID,
		IdempotencyKey: idempotencyKey,
		Status:         domain.BatchStatusAccepted,
		TotalCount:     len(inputs),
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.repo.CreateBatch(ctx, batch, notifications); err != nil {
		return nil, fmt.Errorf("create batch: %w", err)
	}

	return &BatchResult{
		BatchID:    batch.ID.String(),
		Status:     string(batch.Status),
		TotalCount: batch.TotalCount,
		AcceptedAt: batch.CreatedAt,
	}, nil
}

func buildNotifications(batchID uuid.UUID, now time.Time, inputs []NotificationInput) ([]*domain.Notification, error) {
	notifications := make([]*domain.Notification, len(inputs))
	for i, inp := range inputs {
		if inp.Recipient == "" || inp.Channel == "" || inp.Content == "" {
			return nil, &domain.ErrValidation{
				Field:   fmt.Sprintf("notifications[%d]", i),
				Message: "recipient, channel, and content are required",
			}
		}

		if len(inp.Content) > 1024 {
			return nil, &domain.ErrValidation{
				Field:   fmt.Sprintf("notifications[%d].content", i),
				Message: "must not exceed 1024 characters",
			}
		}

		channel := domain.Channel(inp.Channel)
		if !channel.IsValid() {
			return nil, &domain.ErrValidation{
				Field:   fmt.Sprintf("notifications[%d].channel", i),
				Message: fmt.Sprintf("invalid channel '%s'", inp.Channel),
			}
		}

		priority := domain.PriorityNormal
		if inp.Priority != "" {
			priority = domain.Priority(inp.Priority)
			if !priority.IsValid() {
				return nil, &domain.ErrValidation{
					Field:   fmt.Sprintf("notifications[%d].priority", i),
					Message: fmt.Sprintf("invalid priority '%s'", inp.Priority),
				}
			}
		}

		notifications[i] = &domain.Notification{
			ID:        uuid.New(),
			BatchID:   batchID,
			Recipient: inp.Recipient,
			Channel:   channel,
			Content:   inp.Content,
			Priority:  priority,
			Status:    domain.NotificationStatusPending,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}
	return notifications, nil
}