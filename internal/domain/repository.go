package domain

import (
	"context"

	"github.com/google/uuid"
)

type NotificationRepository interface {
	CreateBatch(ctx context.Context, batch *Batch, notifications []*Notification) error
	GetBatchByIdempotencyKey(ctx context.Context, key uuid.UUID) (*Batch, error)
	GetBatchByID(ctx context.Context, id uuid.UUID) (*Batch, error)
	GetNotificationByID(ctx context.Context, id uuid.UUID) (*Notification, error)
	ListNotifications(ctx context.Context, filter NotificationFilter) ([]*Notification, int, error)
	UpdateNotificationStatus(ctx context.Context, id uuid.UUID, status NotificationStatus, errMsg *string, retryCount int) error
	CancelNotification(ctx context.Context, id uuid.UUID) error
	CountByStatus(ctx context.Context) (map[string]int, error)
}