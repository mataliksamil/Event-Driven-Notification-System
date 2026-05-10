package notification

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/samil/notification/internal/adapter/httputil"
	"github.com/samil/notification/internal/domain"
	"github.com/samil/notification/internal/service"
)

type Handler struct {
	notificationSvc *service.NotificationService
}

func NewHandler(notificationSvc *service.NotificationService) *Handler {
	return &Handler{notificationSvc: notificationSvc}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	r.Get("/{id}", h.Get)
	r.Post("/{id}/cancel", h.Cancel)
	return r
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid notification id")
		return
	}

	result, err := h.notificationSvc.GetNotification(r.Context(), id)
	if err != nil {
		var notFoundErr *domain.ErrNotFound
		if errors.As(err, &notFoundErr) {
			httputil.WriteError(w, http.StatusNotFound, notFoundErr.Error())
			return
		}
		slog.Error("internal error getting notification", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, toNotificationResponse(result))
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	params, err := parseListQueryParams(r)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	filter := domain.NotificationFilter{
		BatchID:   params.BatchID,
		StartDate: params.StartDate,
		EndDate:   params.EndDate,
		Page:      params.Page,
		Limit:     params.Limit,
	}

	if params.Status != nil {
		s := domain.NotificationStatus(*params.Status)
		filter.Status = &s
	}
	if params.Channel != nil {
		c := domain.Channel(*params.Channel)
		filter.Channel = &c
	}

	result, err := h.notificationSvc.ListNotifications(r.Context(), filter)
	if err != nil {
		slog.Error("internal error listing notifications", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	data := make([]notificationResponse, len(result.Notifications))
	for i, n := range result.Notifications {
		data[i] = toNotificationResponse(&n)
	}

	httputil.WriteJSON(w, http.StatusOK, paginatedNotificationList{
		Data: data,
		Meta: paginationMeta{
			Total: result.Total,
			Page:  result.Page,
			Limit: result.Limit,
		},
	})
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid notification id")
		return
	}

	err = h.notificationSvc.CancelNotification(r.Context(), id)
	if err != nil {
		var notFoundErr *domain.ErrNotFound
		if errors.As(err, &notFoundErr) {
			httputil.WriteError(w, http.StatusNotFound, notFoundErr.Error())
			return
		}
		var notCancellableErr *domain.ErrNotCancellable
		if errors.As(err, &notCancellableErr) {
			httputil.WriteError(w, http.StatusBadRequest, notCancellableErr.Error())
			return
		}
		slog.Error("internal error cancelling notification", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func parseListQueryParams(r *http.Request) (*listQueryParams, error) {
	q := r.URL.Query()
	params := &listQueryParams{
		Page:  1,
		Limit: 50,
	}

	if s := q.Get("status"); s != "" {
		status := domain.NotificationStatus(s)
		if !status.IsValid() {
			return nil, errors.New("invalid status filter value")
		}
		params.Status = &s
	}

	if c := q.Get("channel"); c != "" {
		channel := domain.Channel(c)
		if !channel.IsValid() {
			return nil, errors.New("invalid channel filter value")
		}
		params.Channel = &c
	}

	if bid := q.Get("batch_id"); bid != "" {
		id, err := uuid.Parse(bid)
		if err != nil {
			return nil, errors.New("invalid batch_id filter value")
		}
		params.BatchID = &id
	}

	if sd := q.Get("start_date"); sd != "" {
		t, err := time.Parse(time.RFC3339, sd)
		if err != nil {
			return nil, errors.New("invalid start_date filter value, must be RFC3339")
		}
		params.StartDate = &t
	}

	if ed := q.Get("end_date"); ed != "" {
		t, err := time.Parse(time.RFC3339, ed)
		if err != nil {
			return nil, errors.New("invalid end_date filter value, must be RFC3339")
		}
		params.EndDate = &t
	}

	if p := q.Get("page"); p != "" {
		page, err := strconv.Atoi(p)
		if err != nil || page < 1 {
			return nil, errors.New("invalid page value")
		}
		params.Page = page
	}

	if l := q.Get("limit"); l != "" {
		limit, err := strconv.Atoi(l)
		if err != nil || limit < 1 || limit > 100 {
			return nil, errors.New("invalid limit value, must be between 1 and 100")
		}
		params.Limit = limit
	}

	return params, nil
}

func toNotificationResponse(r *service.NotificationResult) notificationResponse {
	return notificationResponse{
		ID:           r.ID,
		BatchID:      r.BatchID,
		Recipient:    r.Recipient,
		Channel:      r.Channel,
		Content:      r.Content,
		Priority:     r.Priority,
		Status:       r.Status,
		ErrorMessage: r.ErrorMessage,
		RetryCount:   r.RetryCount,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}
