package domain

import (
	"time"

	"github.com/google/uuid"
)

type Batch struct {
	ID             uuid.UUID
	IdempotencyKey uuid.UUID
	Status         BatchStatus
	TotalCount     int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Notification struct {
	ID        uuid.UUID
	BatchID   uuid.UUID
	Recipient string
	Channel   Channel
	Content   string
	Priority  Priority
	Status    NotificationStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}