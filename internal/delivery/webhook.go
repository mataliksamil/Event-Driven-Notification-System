package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/samil/notification/internal/domain"
)

type RestError struct {
	StatusCode int
	Message    string
}

func (e *RestError) Error() string {
	return fmt.Sprintf("rest error (status %d): %s", e.StatusCode, e.Message)
}

type ErrNetworkFailure struct {
	Err error
}

func (e *ErrNetworkFailure) Error() string {
	return fmt.Sprintf("network failure: %v", e.Err)
}

func (e *ErrNetworkFailure) Unwrap() error {
	return e.Err
}

func IsPermanentFailure(err error) bool {
	var restErr *RestError
	if !errors.As(err, &restErr) {
		return false
	}
	if restErr.StatusCode == http.StatusTooManyRequests {
		return false
	}
	return restErr.StatusCode >= 400 && restErr.StatusCode < 500
}

func IsTemporaryFailure(err error) bool {
	var restErr *RestError
	if errors.As(err, &restErr) {
		return restErr.StatusCode >= 500 || restErr.StatusCode == http.StatusTooManyRequests
	}
	var netErr *ErrNetworkFailure
	return errors.As(err, &netErr)
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

	log.Printf("[WEBHOOK] sending %s %s body=%s", req.Method, c.webhookURL, string(body))

	resp, err := c.client.Do(req)
	if err != nil {
		return &ErrNetworkFailure{Err: fmt.Errorf("http request: %w", err)}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[WEBHOOK] response status=%d body=%s", resp.StatusCode, string(respBody))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return &RestError{
			StatusCode: resp.StatusCode,
			Message:    resp.Status,
		}
	}

	if resp.StatusCode >= 500 {
		return &RestError{
			StatusCode: resp.StatusCode,
			Message:    resp.Status,
		}
	}

	return &RestError{
		StatusCode: resp.StatusCode,
		Message:    fmt.Sprintf("unexpected status: %s", resp.Status),
	}
}