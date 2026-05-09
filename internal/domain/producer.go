package domain

import "context"

type NotificationProducer interface {
	Enqueue(ctx context.Context, notification *Notification) error
}