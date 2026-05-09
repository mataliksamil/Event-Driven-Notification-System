package worker

import "github.com/google/uuid"

type NotificationPayload struct {
	NotificationID uuid.UUID `json:"notification_id"`
}