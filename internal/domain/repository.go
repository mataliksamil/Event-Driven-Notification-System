package domain

import (
	"context"

	"github.com/google/uuid"
)

type NotificationRepository interface {
	CreateBatch(ctx context.Context, batch *Batch, notifications []*Notification) error
	GetBatchByIdempotencyKey(ctx context.Context, key uuid.UUID) (*Batch, error)
	GetNotificationByID(ctx context.Context, id uuid.UUID) (*Notification, error)
	UpdateNotificationStatus(ctx context.Context, id uuid.UUID, status NotificationStatus, errMsg *string, retryCount int) error
}