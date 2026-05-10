package batch

import (
	"time"

	"github.com/google/uuid"
)

type createBatchRequest struct {
	Notifications []notificationPayload `json:"notifications"`
}

type notificationPayload struct {
	Recipient string `json:"recipient"`
	Channel   string `json:"channel"`
	Content   string `json:"content"`
	Priority  string `json:"priority"`
}

type batchResponse struct {
	BatchID    string    `json:"batch_id"`
	Status     string    `json:"status"`
	TotalCount int       `json:"total_count"`
	AcceptedAt time.Time `json:"accepted_at"`
}

type batchQueryResponse struct {
	BatchID       uuid.UUID `json:"batch_id"`
	IdempotencyKey uuid.UUID `json:"idempotency_key"`
	Status        string    `json:"status"`
	TotalCount    int       `json:"total_count"`
	CreatedAt     string    `json:"created_at"`
	UpdatedAt     string    `json:"updated_at"`
}