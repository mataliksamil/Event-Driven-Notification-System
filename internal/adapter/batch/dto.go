package batch

import "time"

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