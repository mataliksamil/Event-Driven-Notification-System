package batch

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/samil/notification/internal/adapter/httputil"
	"github.com/samil/notification/internal/domain"
	redisSvc "github.com/samil/notification/internal/redis"
)

type Handler struct {
	repo        domain.NotificationRepository
	idempotency *redisSvc.IdempotencyService
}

func NewHandler(repo domain.NotificationRepository, idempotency *redisSvc.IdempotencyService) *Handler {
	return &Handler{repo: repo, idempotency: idempotency}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.CreateBatch)
	return r
}

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

func (h *Handler) CreateBatch(w http.ResponseWriter, r *http.Request) {
	idempotencyKeyStr := r.Header.Get("Idempotency-Key")
	if idempotencyKeyStr == "" {
		httputil.WriteError(w, http.StatusBadRequest, "missing Idempotency-Key header")
		return
	}

	idempotencyKey, err := uuid.Parse(idempotencyKeyStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid Idempotency-Key: must be a valid UUID")
		return
	}

	isNew, err := h.idempotency.CheckAndSet(r.Context(), idempotencyKeyStr)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "idempotency check failed")
		return
	}

	if !isNew {
		existing, err := h.repo.GetBatchByIdempotencyKey(r.Context(), idempotencyKey)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to retrieve existing batch")
			return
		}
		httputil.WriteJSON(w, http.StatusAccepted, batchResponse{
			BatchID:    existing.ID.String(),
			Status:     string(existing.Status),
			TotalCount: existing.TotalCount,
			AcceptedAt: existing.CreatedAt,
		})
		return
	}

	var req createBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.idempotency.Release(r.Context(), idempotencyKeyStr)
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if len(req.Notifications) == 0 {
		h.idempotency.Release(r.Context(), idempotencyKeyStr)
		httputil.WriteError(w, http.StatusBadRequest, "notifications array must not be empty")
		return
	}

	if len(req.Notifications) > 1000 {
		h.idempotency.Release(r.Context(), idempotencyKeyStr)
		httputil.WriteError(w, http.StatusBadRequest, "notifications array must not exceed 1000 items")
		return
	}

	batchID := uuid.New()
	now := time.Now().UTC()

	batch := &domain.Batch{
		ID:             batchID,
		IdempotencyKey: idempotencyKey,
		Status:         domain.BatchStatusAccepted,
		TotalCount:     len(req.Notifications),
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	notifications := make([]*domain.Notification, len(req.Notifications))
	for i, np := range req.Notifications {
		if np.Recipient == "" || np.Channel == "" || np.Content == "" {
			h.idempotency.Release(r.Context(), idempotencyKeyStr)
			httputil.WriteError(w, http.StatusBadRequest, fmt.Sprintf("notification[%d]: recipient, channel, and content are required", i))
			return
		}

		if len(np.Content) > 1024 {
			h.idempotency.Release(r.Context(), idempotencyKeyStr)
			httputil.WriteError(w, http.StatusBadRequest, fmt.Sprintf("notification[%d]: content must not exceed 1024 characters", i))
			return
		}

		channel := domain.Channel(np.Channel)
		if !isValidChannel(channel) {
			h.idempotency.Release(r.Context(), idempotencyKeyStr)
			httputil.WriteError(w, http.StatusBadRequest, fmt.Sprintf("notification[%d]: invalid channel '%s'", i, np.Channel))
			return
		}

		priority := domain.PriorityNormal
		if np.Priority != "" {
			priority = domain.Priority(np.Priority)
			if !isValidPriority(priority) {
				h.idempotency.Release(r.Context(), idempotencyKeyStr)
				httputil.WriteError(w, http.StatusBadRequest, fmt.Sprintf("notification[%d]: invalid priority '%s'", i, np.Priority))
				return
			}
		}

		notifications[i] = &domain.Notification{
			ID:        uuid.New(),
			BatchID:   batchID,
			Recipient: np.Recipient,
			Channel:   channel,
			Content:   np.Content,
			Priority:  priority,
			Status:    domain.NotificationStatusPending,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}

	if err := h.repo.CreateBatch(r.Context(), batch, notifications); err != nil {
		h.idempotency.Release(r.Context(), idempotencyKeyStr)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to create batch")
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, batchResponse{
		BatchID:    batchID.String(),
		Status:     string(domain.BatchStatusAccepted),
		TotalCount: len(notifications),
		AcceptedAt: now,
	})
}

func isValidChannel(c domain.Channel) bool {
	switch c {
	case domain.ChannelSMS, domain.ChannelEmail, domain.ChannelPush:
		return true
	}
	return false
}

func isValidPriority(p domain.Priority) bool {
	switch p {
	case domain.PriorityHigh, domain.PriorityNormal, domain.PriorityLow:
		return true
	}
	return false
}