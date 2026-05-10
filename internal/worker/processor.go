package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/samil/notification/internal/delivery"
	"github.com/samil/notification/internal/domain"
	"github.com/samil/notification/internal/logger"
	"github.com/samil/notification/internal/metrics"
	"golang.org/x/time/rate"
)

const rateLimitPerSecond = 100
const maxRetries = 4

type NotificationProcessor struct {
	repo     domain.NotificationRepository
	client   domain.DeliveryClient
	limiters map[domain.Channel]*rate.Limiter
}

func NewNotificationProcessor(repo domain.NotificationRepository, client domain.DeliveryClient) *NotificationProcessor {
	limiters := map[domain.Channel]*rate.Limiter{
		domain.ChannelSMS:   rate.NewLimiter(rateLimitPerSecond, rateLimitPerSecond),
		domain.ChannelEmail: rate.NewLimiter(rateLimitPerSecond, rateLimitPerSecond),
		domain.ChannelPush:  rate.NewLimiter(rateLimitPerSecond, rateLimitPerSecond),
	}

	return &NotificationProcessor{
		repo:     repo,
		client:   client,
		limiters: limiters,
	}
}

func (p *NotificationProcessor) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload NotificationPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	log := slog.With("task_type", t.Type(), "notification_id", payload.NotificationID)
	ctx = logger.WithAttrs(ctx, "notification_id", payload.NotificationID, "task_type", t.Type())

	if payload.CorrelationID != "" {
		ctx = logger.WithAttrs(ctx, "correlation_id", payload.CorrelationID)
		log = log.With("correlation_id", payload.CorrelationID)
	}

	notification, err := p.repo.GetNotificationByID(ctx, payload.NotificationID)
	if err != nil {
		log.Error("failed to fetch notification", "error", err)
		return fmt.Errorf("get notification %s: %w", payload.NotificationID, err)
	}

	log = log.With(
		"channel", notification.Channel,
		"recipient", notification.Recipient,
		"priority", notification.Priority,
		"batch_id", notification.BatchID,
	)

	if notification.Status == domain.NotificationStatusDelivered ||
		notification.Status == domain.NotificationStatusCancelled {
		log.Info("skipped", "status", notification.Status)
		return nil
	}

	attempt := notification.RetryCount + 1
	log = log.With("attempt", attempt, "max_retries", maxRetries)

	log.Info("processing notification")

	if err := p.repo.UpdateNotificationStatus(ctx, notification.ID, domain.NotificationStatusProcessing, nil, notification.RetryCount); err != nil {
		log.Error("failed to update status to processing", "error", err)
		return fmt.Errorf("update status to processing: %w", err)
	}

	limiter, ok := p.limiters[notification.Channel]
	if !ok {
		errMsg := fmt.Sprintf("unknown channel: %s", notification.Channel)
		_ = p.repo.UpdateNotificationStatus(ctx, notification.ID, domain.NotificationStatusFailed, &errMsg, notification.RetryCount)
		log.Error("unknown channel", "channel", notification.Channel)
		return nil
	}

	rateLimitStart := time.Now()
	if err := limiter.Wait(ctx); err != nil {
		waitDur := time.Since(rateLimitStart)
		metrics.RateLimiterWaitDuration.WithLabelValues(string(notification.Channel)).Observe(waitDur.Seconds())
		log.Error("rate limiter wait failed", "error", err, "wait_duration", waitDur)
		return fmt.Errorf("rate limiter wait: %w", err)
	}
	waitDur := time.Since(rateLimitStart)
	metrics.RateLimiterWaitDuration.WithLabelValues(string(notification.Channel)).Observe(waitDur.Seconds())
	log.Info("rate limiter acquired", "wait_duration_ms", waitDur.Milliseconds())

	ctx = logger.WithAttrs(ctx, "channel", notification.Channel, "attempt", attempt)

	processingStart := time.Now()
	sendErr := p.client.Send(ctx, notification.Recipient, notification.Channel, notification.Content)
	processingDur := time.Since(processingStart)
	metrics.ProcessingDuration.WithLabelValues(string(notification.Channel)).Observe(processingDur.Seconds())

	if sendErr == nil {
		if err := p.repo.UpdateNotificationStatus(ctx, notification.ID, domain.NotificationStatusDelivered, nil, notification.RetryCount); err != nil {
			log.Error("delivered but failed to update DB", "error", err)
		}
		metrics.NotificationsProcessed.WithLabelValues(string(notification.Channel), "delivered").Inc()
		log.Info("delivered")
		return nil
	}

	if delivery.IsPermanentFailure(sendErr) {
		errMsg := sendErr.Error()
		if err := p.repo.UpdateNotificationStatus(ctx, notification.ID, domain.NotificationStatusFailed, &errMsg, notification.RetryCount); err != nil {
			log.Error("permanent failure but failed to update DB", "error", err)
		}
		metrics.NotificationsProcessed.WithLabelValues(string(notification.Channel), "failed_permanent").Inc()
		log.Warn("permanent failure", "error", errMsg)
		return nil
	}

	if delivery.IsTemporaryFailure(sendErr) {
		retryCount := notification.RetryCount + 1
		errMsg := sendErr.Error()
		if retryCount >= maxRetries {
			if err := p.repo.UpdateNotificationStatus(ctx, notification.ID, domain.NotificationStatusFailed, &errMsg, retryCount); err != nil {
				log.Error("max retries exceeded but failed to update DB", "error", err)
			}
			metrics.NotificationsProcessed.WithLabelValues(string(notification.Channel), "failed_max_retries").Inc()
			log.Warn("max retries exceeded, marking as failed", "retry_count", retryCount)
			return nil
		}
		if err := p.repo.UpdateNotificationStatus(ctx, notification.ID, domain.NotificationStatusPending, &errMsg, retryCount); err != nil {
			log.Error("temporary failure but failed to update DB for retry", "error", err)
		}
		metrics.NotificationsProcessed.WithLabelValues(string(notification.Channel), "retried").Inc()
		log.Warn("temporary failure, will retry", "retry_count", retryCount, "error", errMsg)
		return fmt.Errorf("temporary failure for notification %s: %w", notification.ID, sendErr)
	}

	if err := p.repo.UpdateNotificationStatus(ctx, notification.ID, domain.NotificationStatusFailed, strPtr(sendErr.Error()), notification.RetryCount); err != nil {
		log.Error("unexpected error but failed to update DB", "error", err)
	}
	metrics.NotificationsProcessed.WithLabelValues(string(notification.Channel), "failed_unexpected").Inc()
	log.Error("unexpected error, marking as failed", "error", sendErr)
	return nil
}

func strPtr(s string) *string {
	return &s
}