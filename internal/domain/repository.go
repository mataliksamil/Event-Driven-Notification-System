package domain

import "context"

type NotificationRepository interface {
	CreateBatch(ctx context.Context, batch *Batch, notifications []*Notification) error
}