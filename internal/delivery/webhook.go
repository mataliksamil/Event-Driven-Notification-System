package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/samil/notification/internal/domain"
)

type ErrPermanentFailure struct {
	StatusCode int
	Err        error
}

func (e *ErrPermanentFailure) Error() string {
	return fmt.Sprintf("permanent failure (status %d): %v", e.StatusCode, e.Err)
}

func (e *ErrPermanentFailure) Unwrap() error {
	return e.Err
}

type ErrTemporaryFailure struct {
	StatusCode int
	Err        error
}

func (e *ErrTemporaryFailure) Error() string {
	return fmt.Sprintf("temporary failure (status %d): %v", e.StatusCode, e.Err)
}

func (e *ErrTemporaryFailure) Unwrap() error {
	return e.Err
}

func IsPermanentFailure(err error) bool {
	_, ok := err.(*ErrPermanentFailure)
	return ok
}

func IsTemporaryFailure(err error) bool {
	_, ok := err.(*ErrTemporaryFailure)
	return ok
}

type webhookPayload struct {
	Recipient string `json:"recipient"`
	Channel   string `json:"channel"`
	Content   string `json:"content"`
}

type WebhookClient struct {
	client     *http.Client
	webhookURL string
}

func NewWebhookClient(webhookURL string) *WebhookClient {
	return &WebhookClient{
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		webhookURL: webhookURL,
	}
}

func (c *WebhookClient) Send(ctx context.Context, recipient string, channel domain.Channel, content string) error {
	payload := webhookPayload{
		Recipient: recipient,
		Channel:   string(channel),
		Content:   content,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return &ErrTemporaryFailure{
			Err: fmt.Errorf("http request: %w", err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return &ErrPermanentFailure{
			StatusCode: resp.StatusCode,
			Err:        fmt.Errorf("client error: %s", resp.Status),
		}
	}

	if resp.StatusCode >= 500 {
		return &ErrTemporaryFailure{
			StatusCode: resp.StatusCode,
			Err:        fmt.Errorf("server error: %s", resp.Status),
		}
	}

	return &ErrTemporaryFailure{
		StatusCode: resp.StatusCode,
		Err:        fmt.Errorf("unexpected status: %s", resp.Status),
	}
}