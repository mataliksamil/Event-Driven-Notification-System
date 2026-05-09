package batch

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/samil/notification/internal/adapter/httputil"
	"github.com/samil/notification/internal/adapter/middleware"
	"github.com/samil/notification/internal/domain"
	"github.com/samil/notification/internal/service"
)

type Handler struct {
	batchSvc *service.BatchService
}

func NewHandler(batchSvc *service.BatchService) *Handler {
	return &Handler{batchSvc: batchSvc}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.CreateBatch)
	return r
}

func (h *Handler) CreateBatch(w http.ResponseWriter, r *http.Request) {
	idempotencyKey, ok := middleware.IdempotencyKeyFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusBadRequest, "missing Idempotency-Key header")
		return
	}

	var req createBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if len(req.Notifications) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "notifications array must not be empty")
		return
	}

	if len(req.Notifications) > 1000 {
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
			httputil.WriteError(w, http.StatusBadRequest, valErr.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, batchResponse{
		BatchID:    result.BatchID,
		Status:     result.Status,
		TotalCount: result.TotalCount,
		AcceptedAt: result.AcceptedAt,
	})
}