package domain

import (
	"time"

	"github.com/google/uuid"
)

type NotificationFilter struct {
	Status    *NotificationStatus
	Channel   *Channel
	BatchID   *uuid.UUID
	StartDate *time.Time
	EndDate   *time.Time
	Page      int
	Limit     int
}
