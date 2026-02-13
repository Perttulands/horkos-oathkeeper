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

func timePtr(t time.Time) *time.Time {
	return &t
}
