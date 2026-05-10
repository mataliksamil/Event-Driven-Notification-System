package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/samil/notification/internal/domain"
)

type NotificationService struct {
	repo domain.NotificationRepository
}

func NewNotificationService(repo domain.NotificationRepository) *NotificationService {
	return &NotificationService{repo: repo}
}

type NotificationResult struct {
	ID           uuid.UUID
	BatchID      uuid.UUID
	Recipient    string
	Channel      string
	Content      string
	Priority     string
	Status       string
	ErrorMessage *string
	RetryCount   int
	CreatedAt    string
	UpdatedAt    string
}

type BatchQueryResult struct {
	BatchID       uuid.UUID
	IdempotencyKey uuid.UUID
	Status        string
	TotalCount    int
	CreatedAt     string
	UpdatedAt     string
}

type ListResult struct {
	Notifications []NotificationResult
	Total         int
	Page          int
	Limit         int
}

func (s *NotificationService) GetNotification(ctx context.Context, id uuid.UUID) (*NotificationResult, error) {
	log := slog.With("component", "notification_service", "notification_id", id)

	n, err := s.repo.GetNotificationByID(ctx, id)
	if err != nil {
		log.Error("failed to get notification", "error", err)
		return nil, fmt.Errorf("get notification: %w", err)
	}
	if n == nil {
		return nil, &domain.ErrNotFound{Resource: "notification", ID: id.String()}
	}

	return &NotificationResult{
		ID:           n.ID,
		BatchID:      n.BatchID,
		Recipient:    n.Recipient,
		Channel:      string(n.Channel),
		Content:      n.Content,
		Priority:     string(n.Priority),
		Status:       string(n.Status),
		ErrorMessage: n.ErrorMessage,
		RetryCount:   n.RetryCount,
		CreatedAt:    n.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    n.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

func (s *NotificationService) GetBatch(ctx context.Context, batchID uuid.UUID) (*BatchQueryResult, error) {
	log := slog.With("component", "notification_service", "batch_id", batchID)

	b, err := s.repo.GetBatchByID(ctx, batchID)
	if err != nil {
		log.Error("failed to get batch", "error", err)
		return nil, fmt.Errorf("get batch: %w", err)
	}
	if b == nil {
		return nil, &domain.ErrNotFound{Resource: "batch", ID: batchID.String()}
	}

	return &BatchQueryResult{
		BatchID:       b.ID,
		IdempotencyKey: b.IdempotencyKey,
		Status:        string(b.Status),
		TotalCount:    b.TotalCount,
		CreatedAt:     b.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     b.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

func (s *NotificationService) ListNotifications(ctx context.Context, filter domain.NotificationFilter) (*ListResult, error) {
	log := slog.With("component", "notification_service")

	notifications, total, err := s.repo.ListNotifications(ctx, filter)
	if err != nil {
		log.Error("failed to list notifications", "error", err)
		return nil, fmt.Errorf("list notifications: %w", err)
	}

	results := make([]NotificationResult, len(notifications))
	for i, n := range notifications {
		results[i] = NotificationResult{
			ID:           n.ID,
			BatchID:      n.BatchID,
			Recipient:    n.Recipient,
			Channel:      string(n.Channel),
			Content:      n.Content,
			Priority:     string(n.Priority),
			Status:       string(n.Status),
			ErrorMessage: n.ErrorMessage,
			RetryCount:   n.RetryCount,
			CreatedAt:    n.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:    n.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	return &ListResult{
		Notifications: results,
		Total:         total,
		Page:          filter.Page,
		Limit:         filter.Limit,
	}, nil
}

func (s *NotificationService) CancelNotification(ctx context.Context, id uuid.UUID) error {
	log := slog.With("component", "notification_service", "notification_id", id)

	err := s.repo.CancelNotification(ctx, id)
	if err != nil {
		log.Error("failed to cancel notification", "error", err)
		return err
	}

	log.Info("notification cancelled")
	return nil
}
