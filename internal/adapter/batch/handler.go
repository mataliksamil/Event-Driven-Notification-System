package batch

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/samil/notification/internal/adapter/httputil"
	"github.com/samil/notification/internal/adapter/middleware"
	"github.com/samil/notification/internal/domain"
	"github.com/samil/notification/internal/service"
)

type Handler struct {
	batchSvc        *service.BatchService
	notificationSvc *service.NotificationService
}

func NewHandler(batchSvc *service.BatchService, notificationSvc *service.NotificationService) *Handler {
	return &Handler{batchSvc: batchSvc, notificationSvc: notificationSvc}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.CreateBatch)
	r.Get("/{batchId}", h.GetBatch)
	return r
}

func (h *Handler) CreateBatch(w http.ResponseWriter, r *http.Request) {
	idempotencyKey, ok := middleware.IdempotencyKeyFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusBadRequest, "missing Idempotency-Key header")
		return
	}

	log := slog.With("component", "batch_handler", "idempotency_key", idempotencyKey)
	log.Info("received batch creation request")

	var req createBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Warn("invalid JSON body", "error", err)
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if len(req.Notifications) == 0 {
		log.Warn("empty notifications array")
		httputil.WriteError(w, http.StatusBadRequest, "notifications array must not be empty")
		return
	}

	if len(req.Notifications) > 1000 {
		log.Warn("notifications array too large", "count", len(req.Notifications))
		httputil.WriteError(w, http.StatusBadRequest, "notifications array must not exceed 1000 items")
		return
	}

	inputs := make([]service.NotificationInput, len(req.Notifications))
	for i, np := range req.Notifications {
		inputs[i] = service.NotificationInput{
			Recipient: np.Recipient,
			Channel:   np.Channel,
			Content:   np.Content,
			Priority:  np.Priority,
		}
	}

	result, err := h.batchSvc.CreateBatch(r.Context(), idempotencyKey, inputs)
	if err != nil {
		var valErr *domain.ErrValidation
		if errors.As(err, &valErr) {
			log.Warn("validation error", "error", valErr)
			httputil.WriteError(w, http.StatusBadRequest, valErr.Error())
			return
		}
		log.Error("internal error creating batch", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	log.Info("batch created", "batch_id", result.BatchID, "total_count", result.TotalCount)
	httputil.WriteJSON(w, http.StatusAccepted, batchResponse{
		BatchID:    result.BatchID,
		Status:     result.Status,
		TotalCount: result.TotalCount,
		AcceptedAt: result.AcceptedAt,
	})
}

func (h *Handler) GetBatch(w http.ResponseWriter, r *http.Request) {
	batchIDStr := chi.URLParam(r, "batchId")
	batchID, err := uuid.Parse(batchIDStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid batch id")
		return
	}

	result, err := h.notificationSvc.GetBatch(r.Context(), batchID)
	if err != nil {
		var notFoundErr *domain.ErrNotFound
		if errors.As(err, &notFoundErr) {
			httputil.WriteError(w, http.StatusNotFound, notFoundErr.Error())
			return
		}
		slog.Error("internal error getting batch", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, batchQueryResponse{
		BatchID:       result.BatchID,
		IdempotencyKey: result.IdempotencyKey,
		Status:        result.Status,
		TotalCount:    result.TotalCount,
		CreatedAt:     result.CreatedAt,
		UpdatedAt:     result.UpdatedAt,
	})
}