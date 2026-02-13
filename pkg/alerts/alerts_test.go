package alerts

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWakeAlerter_SendsPostToCorrectEndpoint(t *testing.T) {
	var receivedPath string
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	alerter := NewWakeAlerter(server.URL)
	commitment := Commitment{
		ID:         "abc123",
		Text:       "I'll check back in 5 minutes",
		Category:   "temporal",
		DetectedAt: time.Date(2026, 2, 13, 14, 30, 22, 0, time.UTC),
		Source:     "athena-01",
	}
	result := VerificationInfo{
		CheckedSources: []string{"crons", "beads"},
		Mechanisms:     []string{},
	}

	err := alerter.SendWakeEvent(commitment, result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedMethod != "POST" {
		t.Errorf("expected POST, got %s", receivedMethod)
	}
	if receivedPath != "/api/v1/sessions/athena-01/wake" {
		t.Errorf("expected /api/v1/sessions/athena-01/wake, got %s", receivedPath)
	}
}

func TestWakeAlerter_SendsCorrectPayload(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	alerter := NewWakeAlerter(server.URL)
	commitment := Commitment{
		ID:         "abc123",
		Text:       "I'll check back in 5 minutes",
		Category:   "temporal",
		DetectedAt: time.Date(2026, 2, 13, 14, 30, 22, 0, time.UTC),
		ExpiresAt:  timePtr(time.Date(2026, 2, 13, 14, 35, 22, 0, time.UTC)),
		Source:     "athena-01",
	}
	result := VerificationInfo{
		CheckedSources: []string{"crons"},
		Mechanisms:     []string{},
	}

	err := alerter.SendWakeEvent(commitment, result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload WakeEventPayload
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}

	if payload.EventType != "oathkeeper_alert" {
		t.Errorf("expected event_type oathkeeper_alert, got %s", payload.EventType)
	}
	if payload.CommitmentID != "abc123" {
		t.Errorf("expected commitment_id abc123, got %s", payload.CommitmentID)
	}
	if payload.Message != "Unverified commitment detected" {
		t.Errorf("unexpected message: %s", payload.Message)
	}
	if payload.Details.Text != "I'll check back in 5 minutes" {
		t.Errorf("unexpected text: %s", payload.Details.Text)
	}
	if payload.Details.Category != "temporal" {
		t.Errorf("unexpected category: %s", payload.Details.Category)
	}
	if payload.Details.DetectedAt != "2026-02-13T14:30:22Z" {
		t.Errorf("unexpected detected_at: %s", payload.Details.DetectedAt)
	}
	if payload.Details.ExpiresAt != "2026-02-13T14:35:22Z" {
		t.Errorf("unexpected expires_at: %s", payload.Details.ExpiresAt)
	}
	if len(payload.Details.Checked) != 1 || payload.Details.Checked[0] != "crons" {
		t.Errorf("unexpected checked: %v", payload.Details.Checked)
	}
	if len(payload.Details.Found) != 0 {
		t.Errorf("expected empty found, got %v", payload.Details.Found)
	}
	if payload.Details.SuggestedAction == "" {
		t.Error("expected non-empty suggested_action")
	}
}

func TestWakeAlerter_NoExpiresAt(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	alerter := NewWakeAlerter(server.URL)
	commitment := Commitment{
		ID:         "def456",
		Text:       "I'll follow up on this",
		Category:   "followup",
		DetectedAt: time.Date(2026, 2, 13, 14, 30, 22, 0, time.UTC),
		ExpiresAt:  nil,
		Source:     "athena-02",
	}
	result := VerificationInfo{
		CheckedSources: []string{"crons"},
		Mechanisms:     []string{},
	}

	err := alerter.SendWakeEvent(commitment, result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload WakeEventPayload
	json.Unmarshal(receivedBody, &payload)

	if payload.Details.ExpiresAt != "" {
		t.Errorf("expected empty expires_at for nil ExpiresAt, got %s", payload.Details.ExpiresAt)
	}
}

func TestWakeAlerter_IncludesFoundMechanisms(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	alerter := NewWakeAlerter(server.URL)
	commitment := Commitment{
		ID:         "ghi789",
		Text:       "I'll monitor the build",
		Category:   "followup",
		DetectedAt: time.Now(),
		Source:     "athena-01",
	}
	result := VerificationInfo{
		CheckedSources: []string{"crons", "beads"},
		Mechanisms:     []string{"cron:abc123"},
	}

	alerter.SendWakeEvent(commitment, result)

	var payload WakeEventPayload
	json.Unmarshal(receivedBody, &payload)

	if len(payload.Details.Found) != 1 || payload.Details.Found[0] != "cron:abc123" {
		t.Errorf("expected found=[cron:abc123], got %v", payload.Details.Found)
	}
}

func TestWakeAlerter_APIErrorReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	alerter := NewWakeAlerter(server.URL)
	commitment := Commitment{
		ID:         "abc123",
		Text:       "I'll check back",
		Category:   "temporal",
		DetectedAt: time.Now(),
		Source:     "athena-01",
	}
	result := VerificationInfo{
		CheckedSources: []string{"crons"},
		Mechanisms:     []string{},
	}

	err := alerter.SendWakeEvent(commitment, result)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestWakeAlerter_UnreachableReturnsError(t *testing.T) {
	alerter := NewWakeAlerter("http://localhost:1")
	commitment := Commitment{
		ID:         "abc123",
		Text:       "I'll check back",
		Category:   "temporal",
		DetectedAt: time.Now(),
		Source:     "athena-01",
	}
	result := VerificationInfo{
		CheckedSources: []string{"crons"},
		Mechanisms:     []string{},
	}

	err := alerter.SendWakeEvent(commitment, result)
	if err == nil {
		t.Error("expected error for unreachable server")
	}
}

func TestWakeAlerter_SetsContentTypeJSON(t *testing.T) {
	var receivedContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	alerter := NewWakeAlerter(server.URL)
	commitment := Commitment{
		ID:         "abc123",
		Text:       "I'll check back",
		Category:   "temporal",
		DetectedAt: time.Now(),
		Source:     "athena-01",
	}
	result := VerificationInfo{
		CheckedSources: []string{"crons"},
		Mechanisms:     []string{},
	}

	alerter.SendWakeEvent(commitment, result)

	if receivedContentType != "application/json" {
		t.Errorf("expected application/json, got %s", receivedContentType)
	}
}

func TestWakeAlerter_SetTimeout(t *testing.T) {
	alerter := NewWakeAlerter("http://localhost:8080")
	alerter.SetTimeout(10 * time.Second)
	// Verify it doesn't panic and sets correctly
	if alerter.client.Timeout != 10*time.Second {
		t.Errorf("expected 10s timeout, got %v", alerter.client.Timeout)
	}
}

func TestWakeAlerter_SessionPathEncoding(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	alerter := NewWakeAlerter(server.URL)
	commitment := Commitment{
		ID:         "abc123",
		Text:       "I'll check back",
		Category:   "temporal",
		DetectedAt: time.Now(),
		Source:     "session-with-dashes-123",
	}
	result := VerificationInfo{
		CheckedSources: []string{"crons"},
		Mechanisms:     []string{},
	}

	alerter.SendWakeEvent(commitment, result)

	if receivedPath != "/api/v1/sessions/session-with-dashes-123/wake" {
		t.Errorf("unexpected path: %s", receivedPath)
	}
}

func TestWakeEventPayload_JSONStructure(t *testing.T) {
	payload := WakeEventPayload{
		EventType:    "oathkeeper_alert",
		CommitmentID: "test123",
		Message:      "Unverified commitment detected",
		Details: WakeEventDetails{
			Text:            "I'll check in 5 minutes",
			Category:        "temporal",
			DetectedAt:      "2026-02-13T14:30:22Z",
			ExpiresAt:       "2026-02-13T14:35:22Z",
			Checked:         []string{"crons", "beads"},
			Found:           []string{},
			SuggestedAction: "Create a mechanism",
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)

	if parsed["event_type"] != "oathkeeper_alert" {
		t.Errorf("expected event_type key in JSON")
	}
	if parsed["commitment_id"] != "test123" {
		t.Errorf("expected commitment_id key in JSON")
	}
	details, ok := parsed["details"].(map[string]interface{})
	if !ok {
		t.Fatal("expected details object in JSON")
	}
	if details["suggested_action"] != "Create a mechanism" {
		t.Errorf("expected suggested_action key in JSON details")
	}
}

// --- TelegramAlerter Tests ---

func TestTelegramAlerter_SendsPostToWebhook(t *testing.T) {
	var receivedMethod string
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	alerter := NewTelegramAlerter(server.URL + "/webhook/telegram")
	commitment := Commitment{
		ID:         "abc123",
		Text:       "I'll check back in 5 minutes",
		Category:   "temporal",
		DetectedAt: time.Date(2026, 2, 13, 14, 30, 22, 0, time.UTC),
		Source:     "athena-01",
	}
	info := VerificationInfo{
		CheckedSources: []string{"crons", "beads"},
		Mechanisms:     []string{},
	}

	err := alerter.SendNotification(commitment, info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedMethod != "POST" {
		t.Errorf("expected POST, got %s", receivedMethod)
	}
	if receivedPath != "/webhook/telegram" {
		t.Errorf("expected /webhook/telegram, got %s", receivedPath)
	}
}

func TestTelegramAlerter_SendsCorrectPayload(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	alerter := NewTelegramAlerter(server.URL + "/webhook/telegram")
	commitment := Commitment{
		ID:         "abc123",
		Text:       "I'll check back in 5 minutes",
		Category:   "temporal",
		DetectedAt: time.Date(2026, 2, 13, 14, 30, 22, 0, time.UTC),
		ExpiresAt:  timePtr(time.Date(2026, 2, 13, 14, 35, 22, 0, time.UTC)),
		Source:     "athena-01",
	}
	info := VerificationInfo{
		CheckedSources: []string{"crons", "beads"},
		Mechanisms:     []string{},
	}

	err := alerter.SendNotification(commitment, info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload TelegramPayload
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}

	if payload.Source != "oathkeeper" {
		t.Errorf("expected source oathkeeper, got %s", payload.Source)
	}
	if payload.Level != "warning" {
		t.Errorf("expected level warning, got %s", payload.Level)
	}
	if payload.CommitmentID != "abc123" {
		t.Errorf("expected commitment_id abc123, got %s", payload.CommitmentID)
	}
	if payload.Category != "temporal" {
		t.Errorf("expected category temporal, got %s", payload.Category)
	}
	if payload.Session != "athena-01" {
		t.Errorf("expected session athena-01, got %s", payload.Session)
	}
	// Message should contain the commitment text
	if payload.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestTelegramAlerter_MessageContainsCommitmentText(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	alerter := NewTelegramAlerter(server.URL + "/webhook/telegram")
	commitment := Commitment{
		ID:         "abc123",
		Text:       "I'll monitor the build process",
		Category:   "followup",
		DetectedAt: time.Now(),
		Source:     "athena-01",
	}
	info := VerificationInfo{
		CheckedSources: []string{"crons"},
		Mechanisms:     []string{},
	}

	alerter.SendNotification(commitment, info)

	var payload TelegramPayload
	json.Unmarshal(receivedBody, &payload)

	// Message should contain the commitment text for user context
	if !containsSubstring(payload.Message, "I'll monitor the build process") {
		t.Errorf("message should contain commitment text, got: %s", payload.Message)
	}
}

func TestTelegramAlerter_NoExpiresAt(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	alerter := NewTelegramAlerter(server.URL + "/webhook/telegram")
	commitment := Commitment{
		ID:         "abc123",
		Text:       "I'll follow up on this",
		Category:   "followup",
		DetectedAt: time.Now(),
		ExpiresAt:  nil,
		Source:     "athena-01",
	}
	info := VerificationInfo{
		CheckedSources: []string{"crons"},
		Mechanisms:     []string{},
	}

	err := alerter.SendNotification(commitment, info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload TelegramPayload
	json.Unmarshal(receivedBody, &payload)

	if payload.ExpiresAt != "" {
		t.Errorf("expected empty expires_at for nil ExpiresAt, got %s", payload.ExpiresAt)
	}
}

func TestTelegramAlerter_WithExpiresAt(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	alerter := NewTelegramAlerter(server.URL + "/webhook/telegram")
	commitment := Commitment{
		ID:         "abc123",
		Text:       "I'll check in 5 minutes",
		Category:   "temporal",
		DetectedAt: time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC),
		ExpiresAt:  timePtr(time.Date(2026, 2, 13, 14, 35, 0, 0, time.UTC)),
		Source:     "athena-01",
	}
	info := VerificationInfo{
		CheckedSources: []string{"crons"},
		Mechanisms:     []string{},
	}

	alerter.SendNotification(commitment, info)

	var payload TelegramPayload
	json.Unmarshal(receivedBody, &payload)

	if payload.ExpiresAt != "2026-02-13T14:35:00Z" {
		t.Errorf("expected expires_at 2026-02-13T14:35:00Z, got %s", payload.ExpiresAt)
	}
}

func TestTelegramAlerter_APIErrorReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	alerter := NewTelegramAlerter(server.URL + "/webhook/telegram")
	commitment := Commitment{
		ID:         "abc123",
		Text:       "I'll check back",
		Category:   "temporal",
		DetectedAt: time.Now(),
		Source:     "athena-01",
	}
	info := VerificationInfo{
		CheckedSources: []string{"crons"},
		Mechanisms:     []string{},
	}

	err := alerter.SendNotification(commitment, info)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestTelegramAlerter_UnreachableReturnsError(t *testing.T) {
	alerter := NewTelegramAlerter("http://localhost:1/webhook/telegram")
	commitment := Commitment{
		ID:         "abc123",
		Text:       "I'll check back",
		Category:   "temporal",
		DetectedAt: time.Now(),
		Source:     "athena-01",
	}
	info := VerificationInfo{
		CheckedSources: []string{"crons"},
		Mechanisms:     []string{},
	}

	err := alerter.SendNotification(commitment, info)
	if err == nil {
		t.Error("expected error for unreachable server")
	}
}

func TestTelegramAlerter_SetsContentTypeJSON(t *testing.T) {
	var receivedContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	alerter := NewTelegramAlerter(server.URL + "/webhook/telegram")
	commitment := Commitment{
		ID:         "abc123",
		Text:       "I'll check back",
		Category:   "temporal",
		DetectedAt: time.Now(),
		Source:     "athena-01",
	}
	info := VerificationInfo{
		CheckedSources: []string{"crons"},
		Mechanisms:     []string{},
	}

	alerter.SendNotification(commitment, info)

	if receivedContentType != "application/json" {
		t.Errorf("expected application/json, got %s", receivedContentType)
	}
}

func TestTelegramAlerter_SetTimeout(t *testing.T) {
	alerter := NewTelegramAlerter("http://localhost:9090/webhook/telegram")
	alerter.SetTimeout(10 * time.Second)
	if alerter.client.Timeout != 10*time.Second {
		t.Errorf("expected 10s timeout, got %v", alerter.client.Timeout)
	}
}

func TestTelegramAlerter_IncludesCheckedSources(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	alerter := NewTelegramAlerter(server.URL + "/webhook/telegram")
	commitment := Commitment{
		ID:         "abc123",
		Text:       "I'll check back",
		Category:   "temporal",
		DetectedAt: time.Now(),
		Source:     "athena-01",
	}
	info := VerificationInfo{
		CheckedSources: []string{"crons", "beads", "state_files"},
		Mechanisms:     []string{},
	}

	alerter.SendNotification(commitment, info)

	var payload TelegramPayload
	json.Unmarshal(receivedBody, &payload)

	if len(payload.Checked) != 3 {
		t.Errorf("expected 3 checked sources, got %d", len(payload.Checked))
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringImpl(s, substr))
}

func containsSubstringImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func timePtr(t time.Time) *time.Time {
	return &t
}
