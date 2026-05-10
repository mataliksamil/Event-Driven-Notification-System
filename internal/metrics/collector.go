package metrics

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

func StartServer(addr string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", Handler())

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("metrics server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server error", "error", err)
		}
	}()
}

func StartQueueCollector(ctx context.Context, inspector QueueInspector, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				collectQueueMetrics(ctx, inspector)
			}
		}
	}()
}

func StartDBCollector(ctx context.Context, repo StatusCounter, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				collectDBMetrics(ctx, repo)
			}
		}
	}()
}

type QueueInspector interface {
	GetQueueInfo(queue string) (*QueueInfo, error)
}

type QueueInfo struct {
	Size     int
	Active   int
	Pending  int
	Retry    int
}

type StatusCounter interface {
	CountByStatus(ctx context.Context) (map[string]int, error)
}

func collectQueueMetrics(ctx context.Context, inspector QueueInspector) {
	queues := []string{"critical", "default", "low"}
	for _, q := range queues {
		info, err := inspector.GetQueueInfo(q)
		if err != nil {
			slog.Warn("failed to get queue info", "queue", q, "error", err)
			continue
		}
		QueueSize.WithLabelValues(q).Set(float64(info.Size))
		QueueActive.WithLabelValues(q).Set(float64(info.Active))
		QueuePending.WithLabelValues(q).Set(float64(info.Pending))
		QueueRetry.WithLabelValues(q).Set(float64(info.Retry))
	}
}

func collectDBMetrics(ctx context.Context, repo StatusCounter) {
	counts, err := repo.CountByStatus(ctx)
	if err != nil {
		slog.Warn("failed to count notifications by status", "error", err)
		return
	}
	for status, count := range counts {
		NotificationsByStatus.WithLabelValues(status).Set(float64(count))
	}
}