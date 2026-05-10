package worker

import "github.com/google/uuid"

type NotificationPayload struct {
	NotificationID uuid.UUID `json:"notification_id"`
	CorrelationID  string    `json:"correlation_id,omitempty"`
}