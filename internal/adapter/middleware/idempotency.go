package middleware

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/samil/notification/internal/adapter/httputil"
	redisSvc "github.com/samil/notification/internal/redis"
)

type contextKey string

const IdempotencyKeyCtx contextKey = "idempotency_key"

func IdempotencyKeyFromContext(ctx context.Context) (uuid.UUID, bool) {
	key, ok := ctx.Value(IdempotencyKeyCtx).(uuid.UUID)
	return key, ok
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

type IdempotencyMiddleware struct {
	idempotency *redisSvc.IdempotencyService
}

func NewIdempotency(idempotency *redisSvc.IdempotencyService) *IdempotencyMiddleware {
	return &IdempotencyMiddleware{idempotency: idempotency}
}

func (m *IdempotencyMiddleware) Handle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keyStr := r.Header.Get("Idempotency-Key")
		if keyStr == "" {
			httputil.WriteError(w, http.StatusBadRequest, "missing Idempotency-Key header")
			return
		}

		key, err := uuid.Parse(keyStr)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid Idempotency-Key: must be a valid UUID")
			return
		}

		log := slog.With("component", "idempotency", "idempotency_key", keyStr)

		result, err := m.idempotency.Acquire(r.Context(), keyStr)
		if err != nil {
			log.Error("idempotency acquire failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "idempotency check failed")
			return
		}

		switch result.Status {
		case redisSvc.StatusCompleted:
			log.Info("returning cached idempotent response")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			w.Write(result.CachedResponse)
			return
		case redisSvc.StatusProcessing:
			log.Info("batch already processing")
			httputil.WriteJSON(w, http.StatusAccepted, map[string]string{
				"status":  "processing",
				"message": "batch is being processed",
			})
			return
		}

		log.Info("new request, proceeding")

		ctx := context.WithValue(r.Context(), IdempotencyKeyCtx, key)

		rec := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rec, r.WithContext(ctx))

		if rec.statusCode >= 400 {
			log.Warn("request failed, releasing idempotency lock", "status_code", rec.statusCode)
			_ = m.idempotency.Release(r.Context(), keyStr)
			return
		}

		if err := m.idempotency.Complete(r.Context(), keyStr, rec.body.Bytes()); err != nil {
			log.Error("failed to complete idempotency", "error", err)
		} else {
			log.Info("idempotency completed")
		}
	})
}