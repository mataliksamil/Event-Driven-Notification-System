package domain

import (
	"context"

	"github.com/google/uuid"
)

type NotificationRepository interface {
	CreateBatch(ctx context.Context, batch *Batch, notifications []*Notification) error
	GetBatchByIdempotencyKey(ctx context.Context, key uuid.UUID) (*Batch, error)
}