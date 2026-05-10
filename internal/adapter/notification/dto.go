package notification

import (
	"time"

	"github.com/google/uuid"
)

type notificationResponse struct {
	ID           uuid.UUID `json:"id"`
	BatchID      uuid.UUID `json:"batch_id"`
	Recipient    string    `json:"recipient"`
	Channel      string    `json:"channel"`
	Content      string    `json:"content"`
	Priority     string    `json:"priority"`
	Status       string    `json:"status"`
	ErrorMessage *string   `json:"error_message,omitempty"`
	RetryCount   int       `json:"retry_count"`
	CreatedAt    string    `json:"created_at"`
	UpdatedAt    string    `json:"updated_at"`
}

type paginatedNotificationList struct {
	Data []notificationResponse `json:"data"`
	Meta paginationMeta         `json:"meta"`
}

type paginationMeta struct {
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

type listQueryParams struct {
	Status    *string
	Channel   *string
	BatchID   *uuid.UUID
	StartDate *time.Time
	EndDate   *time.Time
	Page      int
	Limit     int
}
