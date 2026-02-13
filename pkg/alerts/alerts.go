package alerts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Commitment holds the commitment data needed for alerting
type Commitment struct {
	ID         string
	Text       string
	Category   string
	DetectedAt time.Time
	ExpiresAt  *time.Time
	Source     string // OpenClaw session key
}

// VerificationInfo holds the verification results for alert context
type VerificationInfo struct {
	CheckedSources []string
	Mechanisms     []string
}

// WakeEventPayload is the JSON payload sent to OpenClaw's wake endpoint
type WakeEventPayload struct {
	EventType    string           `json:"event_type"`
	CommitmentID string           `json:"commitment_id"`
	Message      string           `json:"message"`
	Details      WakeEventDetails `json:"details"`
}

// WakeEventDetails contains the detailed commitment info in the alert
type WakeEventDetails struct {
	Text            string   `json:"text"`
	Category        string   `json:"category"`
	DetectedAt      string   `json:"detected_at"`
	ExpiresAt       string   `json:"expires_at,omitempty"`
	Checked         []string `json:"checked"`
	Found           []string `json:"found"`
	SuggestedAction string   `json:"suggested_action"`
}

// TelegramPayload is the JSON payload sent to the Argus webhook for Telegram notifications
type TelegramPayload struct {
	Source       string   `json:"source"`
	Level        string   `json:"level"`
	CommitmentID string   `json:"commitment_id"`
	Message      string   `json:"message"`
	Category     string   `json:"category"`
	Session      string   `json:"session"`
	ExpiresAt    string   `json:"expires_at,omitempty"`
	Checked      []string `json:"checked"`
}

// TelegramAlerter sends notifications to Telegram via the Argus webhook
type TelegramAlerter struct {
	webhookURL string
	client     *http.Client
}

// NewTelegramAlerter creates an alerter that sends Telegram notifications via Argus
func NewTelegramAlerter(webhookURL string) *TelegramAlerter {
	return &TelegramAlerter{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 5 * time.Second},
	}
}

// SetTimeout sets the HTTP client timeout
func (a *TelegramAlerter) SetTimeout(d time.Duration) {
	a.client.Timeout = d
}

// SendNotification sends a Telegram notification about an unbacked commitment via Argus
func (a *TelegramAlerter) SendNotification(commitment Commitment, info VerificationInfo) error {
	var expiresAt string
	if commitment.ExpiresAt != nil {
		expiresAt = commitment.ExpiresAt.UTC().Format(time.RFC3339)
	}

	payload := TelegramPayload{
		Source:       "oathkeeper",
		Level:        "warning",
		CommitmentID: commitment.ID,
		Message:      fmt.Sprintf("Unbacked commitment: %s", commitment.Text),
		Category:     commitment.Category,
		Session:      commitment.Source,
		ExpiresAt:    expiresAt,
		Checked:      info.CheckedSources,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal telegram payload: %w", err)
	}

	req, err := http.NewRequest("POST", a.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// WakeAlerter sends wake events to OpenClaw sessions
type WakeAlerter struct {
	apiURL string
	client *http.Client
}

// NewWakeAlerter creates an alerter that sends wake events to OpenClaw
func NewWakeAlerter(apiURL string) *WakeAlerter {
	return &WakeAlerter{
		apiURL: apiURL,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// SetTimeout sets the HTTP client timeout
func (a *WakeAlerter) SetTimeout(d time.Duration) {
	a.client.Timeout = d
}

// SendWakeEvent sends an alert to the originating OpenClaw session
func (a *WakeAlerter) SendWakeEvent(commitment Commitment, info VerificationInfo) error {
	var expiresAt string
	if commitment.ExpiresAt != nil {
		expiresAt = commitment.ExpiresAt.UTC().Format(time.RFC3339)
	}

	found := info.Mechanisms
	if found == nil {
		found = []string{}
	}

	payload := WakeEventPayload{
		EventType:    "oathkeeper_alert",
		CommitmentID: commitment.ID,
		Message:      "Unverified commitment detected",
		Details: WakeEventDetails{
			Text:            commitment.Text,
			Category:        commitment.Category,
			DetectedAt:      commitment.DetectedAt.UTC().Format(time.RFC3339),
			ExpiresAt:       expiresAt,
			Checked:         info.CheckedSources,
			Found:           found,
			SuggestedAction: "Create a cron job or bead to fulfill this commitment, or clarify that it was not a genuine promise.",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal wake event: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/sessions/%s/wake", a.apiURL, commitment.Source)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create wake request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("wake event request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wake event returned status %d", resp.StatusCode)
	}

	return nil
}
