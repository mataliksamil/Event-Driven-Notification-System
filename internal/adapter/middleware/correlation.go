package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/samil/notification/internal/logger"
)

const HeaderCorrelationID = "X-Correlation-ID"

type correlationIDKey struct{}

func CorrelationIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(correlationIDKey{}).(string); ok {
		return id
	}
	return ""
}

type CorrelationMiddleware struct{}

func NewCorrelation() *CorrelationMiddleware {
	return &CorrelationMiddleware{}
}

func (m *CorrelationMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := r.Header.Get(HeaderCorrelationID)
		if correlationID == "" {
			correlationID = uuid.New().String()
		}

		ctx := context.WithValue(r.Context(), correlationIDKey{}, correlationID)
		ctx = logger.WithAttrs(ctx, "correlation_id", correlationID)

		w.Header().Set(HeaderCorrelationID, correlationID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
