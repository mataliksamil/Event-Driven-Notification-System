package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/hibiken/asynq"
	"github.com/samil/notification/internal/delivery"
	"github.com/samil/notification/internal/domain"
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

	notification, err := p.repo.GetNotificationByID(ctx, payload.NotificationID)
	if err != nil {
		return fmt.Errorf("get notification %s: %w", payload.NotificationID, err)
	}

	if notification.Status == domain.NotificationStatusDelivered ||
		notification.Status == domain.NotificationStatusCancelled {
		log.Printf("[PROCESSOR] notification %s: skipped, already %s", notification.ID, notification.Status)
		return nil
	}

	attempt := notification.RetryCount + 1
	log.Printf("[PROCESSOR] notification %s: attempt %d/%d, sending %s to %s", notification.ID, attempt, maxRetries, notification.Channel, notification.Recipient)

	if err := p.repo.UpdateNotificationStatus(ctx, notification.ID, domain.NotificationStatusProcessing, nil, notification.RetryCount); err != nil {
		return fmt.Errorf("update status to processing: %w", err)
	}

	limiter, ok := p.limiters[notification.Channel]
	if !ok {
		errMsg := fmt.Sprintf("unknown channel: %s", notification.Channel)
		_ = p.repo.UpdateNotificationStatus(ctx, notification.ID, domain.NotificationStatusFailed, &errMsg, notification.RetryCount)
		log.Printf("[PROCESSOR] notification %s: failed, unknown channel %s", notification.ID, notification.Channel)
		return nil
	}

	if err := limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limiter wait: %w", err)
	}

	sendErr := p.client.Send(ctx, notification.Recipient, notification.Channel, notification.Content)

	if sendErr == nil {
		if err := p.repo.UpdateNotificationStatus(ctx, notification.ID, domain.NotificationStatusDelivered, nil, notification.RetryCount); err != nil {
			log.Printf("[PROCESSOR] notification %s: delivered but failed to update DB: %v", notification.ID, err)
		}
		log.Printf("[PROCESSOR] notification %s: delivered successfully", notification.ID)
		return nil
	}

	if delivery.IsPermanentFailure(sendErr) {
		errMsg := sendErr.Error()
		if err := p.repo.UpdateNotificationStatus(ctx, notification.ID, domain.NotificationStatusFailed, &errMsg, notification.RetryCount); err != nil {
			log.Printf("[PROCESSOR] notification %s: permanent failure but failed to update DB: %v", notification.ID, err)
		}
		log.Printf("[PROCESSOR] notification %s: permanent failure (4xx): %s", notification.ID, errMsg)
		return nil
	}

	if delivery.IsTemporaryFailure(sendErr) {
		retryCount := notification.RetryCount + 1
		errMsg := sendErr.Error()
		if retryCount >= maxRetries {
			if err := p.repo.UpdateNotificationStatus(ctx, notification.ID, domain.NotificationStatusFailed, &errMsg, retryCount); err != nil {
				log.Printf("[PROCESSOR] notification %s: max retries exceeded but failed to update DB: %v", notification.ID, err)
			}
			log.Printf("[PROCESSOR] notification %s: max retries (%d) exceeded, marking as failed", notification.ID, maxRetries)
			return nil
		}
		if err := p.repo.UpdateNotificationStatus(ctx, notification.ID, domain.NotificationStatusPending, &errMsg, retryCount); err != nil {
			log.Printf("[PROCESSOR] notification %s: temporary failure but failed to update DB for retry: %v", notification.ID, err)
		}
		log.Printf("[PROCESSOR] notification %s: temporary failure, retry %d/%d in ~%ds: %s", notification.ID, retryCount, maxRetries-1, (retryCount+1)*10, errMsg)
		return fmt.Errorf("temporary failure for notification %s: %w", notification.ID, sendErr)
	}

	if err := p.repo.UpdateNotificationStatus(ctx, notification.ID, domain.NotificationStatusFailed, strPtr(sendErr.Error()), notification.RetryCount); err != nil {
		log.Printf("[PROCESSOR] notification %s: unexpected error but failed to update DB: %v", notification.ID, err)
	}
	log.Printf("[PROCESSOR] notification %s: unexpected error, marking as failed: %s", notification.ID, sendErr.Error())
	return nil
}

func strPtr(s string) *string {
	return &s
}