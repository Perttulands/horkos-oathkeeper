package hooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WebhookEvent represents the type of webhook event.
type WebhookEvent string

const (
	EventUnbacked WebhookEvent = "commitment.unbacked"
	EventResolved WebhookEvent = "commitment.resolved"
)

// WebhookPayload is the JSON body sent to the webhook URL.
type WebhookPayload struct {
	Event      WebhookEvent `json:"event"`
	BeadID     string       `json:"bead_id"`
	Text       string       `json:"text,omitempty"`
	Category   string       `json:"category,omitempty"`
	Evidence   string       `json:"evidence,omitempty"`
	ResolvedAt string       `json:"resolved_at,omitempty"`
}

// Webhook sends event notifications to a configurable URL with retry.
type Webhook struct {
	url        string
	client     *http.Client
	maxRetries int
	baseDelay  time.Duration
}

// NewWebhook creates a Webhook that sends events to the given URL.
func NewWebhook(url string) *Webhook {
	return &Webhook{
		url: url,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		maxRetries: 3,
		baseDelay:  1 * time.Second,
	}
}

// SetBaseDelay overrides the base retry delay (for testing).
func (w *Webhook) SetBaseDelay(d time.Duration) {
	w.baseDelay = d
}

// NotifyUnbacked sends a commitment.unbacked event.
func (w *Webhook) NotifyUnbacked(beadID, text, category string) error {
	return w.send(WebhookPayload{
		Event:    EventUnbacked,
		BeadID:   beadID,
		Text:     text,
		Category: category,
	})
}

// NotifyResolved sends a commitment.resolved event.
func (w *Webhook) NotifyResolved(beadID, evidence string) error {
	return w.send(WebhookPayload{
		Event:      EventResolved,
		BeadID:     beadID,
		Evidence:   evidence,
		ResolvedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

func (w *Webhook) send(payload WebhookPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < w.maxRetries; attempt++ {
		if attempt > 0 {
			delay := w.baseDelay * (1 << (attempt - 1)) // 1s, 2s, 4s
			time.Sleep(delay)
		}

		req, err := http.NewRequest(http.MethodPost, w.url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create webhook request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := w.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("webhook request failed: %w", err)
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}

		lastErr = fmt.Errorf("webhook returned status %d", resp.StatusCode)
		if resp.StatusCode < 500 {
			// Client errors (4xx) are not retryable
			return lastErr
		}
		// Server errors (5xx) — retry
	}

	return fmt.Errorf("webhook failed after %d attempts: %w", w.maxRetries, lastErr)
}
