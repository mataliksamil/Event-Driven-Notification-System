package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	NotificationsProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "notification_processed_total",
		Help: "Total number of notifications processed",
	}, []string{"channel", "status"})

	ProcessingDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "notification_processing_duration_seconds",
		Help:    "Time spent processing a notification end-to-end",
		Buckets: prometheus.DefBuckets,
	}, []string{"channel"})

	RateLimiterWaitDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "notification_rate_limiter_wait_seconds",
		Help:    "Time spent waiting for rate limiter token",
		Buckets: prometheus.DefBuckets,
	}, []string{"channel"})

	WebhookRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "webhook_request_duration_seconds",
		Help:    "Duration of webhook HTTP requests",
		Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"channel", "status_code"})

	WebhookRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "webhook_requests_total",
		Help: "Total number of webhook HTTP requests",
	}, []string{"channel", "status_code"})

	QueueSize = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "asynq_queue_size",
		Help: "Number of tasks in the queue",
	}, []string{"queue"})

	QueueActive = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "asynq_queue_active",
		Help: "Number of actively processing tasks in the queue",
	}, []string{"queue"})

	QueuePending = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "asynq_queue_pending",
		Help: "Number of pending tasks in the queue",
	}, []string{"queue"})

	QueueRetry = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "asynq_queue_retry",
		Help: "Number of tasks awaiting retry in the queue",
	}, []string{"queue"})

	NotificationsByStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "notification_db_count",
		Help: "Number of notifications in the database by status",
	}, []string{"status"})

	BatchesCreatedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "batch_created_total",
		Help: "Total number of batches created",
	})

	NotificationsEnqueuedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "notification_enqueued_total",
		Help: "Total number of notifications enqueued for processing",
	}, []string{"channel", "priority"})

	IdempotencyCacheHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "idempotency_cache_total",
		Help: "Total idempotency lookups by result",
	}, []string{"result"})
)

func Handler() http.Handler {
	return promhttp.Handler()
}