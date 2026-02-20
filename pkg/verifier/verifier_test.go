package verifier

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// --- CronChecker Tests ---

func TestCronChecker_FindsRecentCronJob(t *testing.T) {
	detectedAt := time.Now().Add(-25 * time.Second) // commitment detected 25s ago

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify query parameter
		since := r.URL.Query().Get("since")
		if since == "" {
			t.Error("expected 'since' query parameter")
		}

		// Return a cron job created after detection
		resp := CronAPIResponse{
			Crons: []CronJob{
				{
					ID:        "abc123",
					Schedule:  "*/5 * * * *",
					Command:   "check-build-status",
					CreatedAt: time.Now().Add(-10 * time.Second).Unix(),
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	checker := NewCronChecker(server.URL)
	mechanisms, err := checker.Check(detectedAt)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mechanisms) != 1 {
		t.Fatalf("expected 1 mechanism, got %d", len(mechanisms))
	}
	if mechanisms[0] != "cron:abc123" {
		t.Errorf("expected 'cron:abc123', got '%s'", mechanisms[0])
	}
}

func TestCronChecker_UsesConfiguredEndpoint(t *testing.T) {
	detectedAt := time.Now().Add(-25 * time.Second)
	var seenPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		resp := CronAPIResponse{Crons: []CronJob{}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	checker := NewCronCheckerWithEndpoint(server.URL, "/custom/crons")
	_, err := checker.Check(detectedAt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seenPath != "/custom/crons" {
		t.Fatalf("expected path /custom/crons, got %q", seenPath)
	}
}

func TestCronChecker_NoCronJobs(t *testing.T) {
	detectedAt := time.Now().Add(-25 * time.Second)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := CronAPIResponse{Crons: []CronJob{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	checker := NewCronChecker(server.URL)
	mechanisms, err := checker.Check(detectedAt)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mechanisms) != 0 {
		t.Errorf("expected 0 mechanisms, got %d", len(mechanisms))
	}
}

func TestCronChecker_MultipleCronJobs(t *testing.T) {
	detectedAt := time.Now().Add(-25 * time.Second)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := CronAPIResponse{
			Crons: []CronJob{
				{ID: "abc123", Schedule: "*/5 * * * *", Command: "check-build", CreatedAt: time.Now().Add(-10 * time.Second).Unix()},
				{ID: "def456", Schedule: "*/10 * * * *", Command: "monitor-deploy", CreatedAt: time.Now().Add(-5 * time.Second).Unix()},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	checker := NewCronChecker(server.URL)
	mechanisms, err := checker.Check(detectedAt)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mechanisms) != 2 {
		t.Fatalf("expected 2 mechanisms, got %d", len(mechanisms))
	}
	if mechanisms[0] != "cron:abc123" {
		t.Errorf("expected 'cron:abc123', got '%s'", mechanisms[0])
	}
	if mechanisms[1] != "cron:def456" {
		t.Errorf("expected 'cron:def456', got '%s'", mechanisms[1])
	}
}

func TestCronChecker_FiltersStaleCronJobsLocally(t *testing.T) {
	detectedAt := time.Now().Add(-25 * time.Second)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := CronAPIResponse{
			Crons: []CronJob{
				{ID: "stale", Schedule: "*/5 * * * *", Command: "old", CreatedAt: detectedAt.Add(-1 * time.Minute).Unix()},
				{ID: "fresh", Schedule: "*/5 * * * *", Command: "new", CreatedAt: detectedAt.Add(5 * time.Second).Unix()},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	checker := NewCronChecker(server.URL)
	mechanisms, err := checker.Check(detectedAt)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mechanisms) != 1 {
		t.Fatalf("expected 1 mechanism after local filtering, got %d (%v)", len(mechanisms), mechanisms)
	}
	if mechanisms[0] != "cron:fresh" {
		t.Fatalf("expected cron:fresh, got %v", mechanisms[0])
	}
}

func TestCronChecker_FiltersDisabledCrons(t *testing.T) {
	detectedAt := time.Now().Add(-25 * time.Second)
	enabled := true
	disabled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := CronAPIResponse{
			Crons: []CronJob{
				{ID: "enabled", CreatedAt: detectedAt.Add(5 * time.Second).Unix(), Enabled: &enabled},
				{ID: "disabled", CreatedAt: detectedAt.Add(5 * time.Second).Unix(), Enabled: &disabled},
				{ID: "paused", CreatedAt: detectedAt.Add(5 * time.Second).Unix(), Status: "paused"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	checker := NewCronChecker(server.URL)
	mechanisms, err := checker.Check(detectedAt)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mechanisms) != 1 || mechanisms[0] != "cron:enabled" {
		t.Fatalf("expected only enabled cron, got %v", mechanisms)
	}
}

func TestCronChecker_AcceptsItemsResponseShape(t *testing.T) {
	detectedAt := time.Now().Add(-25 * time.Second)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":         "from-items",
					"created_at": detectedAt.Add(10 * time.Second).Unix(),
				},
			},
		})
	}))
	defer server.Close()

	checker := NewCronChecker(server.URL)
	mechanisms, err := checker.Check(detectedAt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mechanisms) != 1 || mechanisms[0] != "cron:from-items" {
		t.Fatalf("expected cron:from-items from items[] response, got %v", mechanisms)
	}
}

func TestCronChecker_APIError(t *testing.T) {
	detectedAt := time.Now().Add(-25 * time.Second)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	checker := NewCronChecker(server.URL)
	mechanisms, err := checker.Check(detectedAt)

	if err == nil {
		t.Error("expected error for 500 response")
	}
	if len(mechanisms) != 0 {
		t.Errorf("expected 0 mechanisms on error, got %d", len(mechanisms))
	}
}

func TestCronChecker_APIUnreachable(t *testing.T) {
	detectedAt := time.Now().Add(-25 * time.Second)

	checker := NewCronChecker("http://localhost:1") // unreachable port
	mechanisms, err := checker.Check(detectedAt)

	if err == nil {
		t.Error("expected error for unreachable API")
	}
	if len(mechanisms) != 0 {
		t.Errorf("expected 0 mechanisms on error, got %d", len(mechanisms))
	}
}

func TestCronChecker_SendsSinceParameter(t *testing.T) {
	detectedAt := time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC)
	var receivedSince string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSince = r.URL.Query().Get("since")
		resp := CronAPIResponse{Crons: []CronJob{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	checker := NewCronChecker(server.URL)
	checker.Check(detectedAt)

	expected := fmt.Sprintf("%d", detectedAt.Unix())
	if receivedSince != expected {
		t.Errorf("expected since='%s', got '%s'", expected, receivedSince)
	}
}

func TestCronChecker_InvalidJSON(t *testing.T) {
	detectedAt := time.Now().Add(-25 * time.Second)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	checker := NewCronChecker(server.URL)
	mechanisms, err := checker.Check(detectedAt)

	if err == nil {
		t.Error("expected error for invalid JSON")
	}
	if len(mechanisms) != 0 {
		t.Errorf("expected 0 mechanisms on error, got %d", len(mechanisms))
	}
}

// --- Verifier Tests ---

func TestVerifier_BackedByRecentCron(t *testing.T) {
	detectedAt := time.Now().Add(-25 * time.Second)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := CronAPIResponse{
			Crons: []CronJob{
				{ID: "abc123", Schedule: "*/5 * * * *", Command: "check-build", CreatedAt: time.Now().Add(-10 * time.Second).Unix()},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	v := NewVerifier(server.URL)
	result, err := v.Verify(detectedAt)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsBacked {
		t.Error("expected commitment to be backed")
	}
	if len(result.Mechanisms) != 1 {
		t.Fatalf("expected 1 mechanism, got %d", len(result.Mechanisms))
	}
	if result.Mechanisms[0] != "cron:abc123" {
		t.Errorf("expected 'cron:abc123', got '%s'", result.Mechanisms[0])
	}
}

func TestVerifier_NotBacked(t *testing.T) {
	detectedAt := time.Now().Add(-25 * time.Second)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := CronAPIResponse{Crons: []CronJob{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	v := NewVerifier(server.URL)
	result, err := v.Verify(detectedAt)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsBacked {
		t.Error("expected commitment to NOT be backed")
	}
	if len(result.Mechanisms) != 0 {
		t.Errorf("expected 0 mechanisms, got %d", len(result.Mechanisms))
	}
}

func TestVerifier_CheckedSourcesIncludesCrons(t *testing.T) {
	detectedAt := time.Now().Add(-25 * time.Second)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := CronAPIResponse{Crons: []CronJob{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	v := NewVerifier(server.URL)
	result, err := v.Verify(detectedAt)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, source := range result.CheckedSources {
		if source == "crons" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'crons' in checked sources, got %v", result.CheckedSources)
	}
}

func TestVerifier_GracefulDegradation(t *testing.T) {
	// Verifier should not fail entirely if cron API is unreachable
	detectedAt := time.Now().Add(-25 * time.Second)

	v := NewVerifier("http://localhost:1") // unreachable
	result, err := v.Verify(detectedAt)

	// Should NOT return an error — graceful degradation
	if err != nil {
		t.Fatalf("verifier should degrade gracefully, got error: %v", err)
	}
	if result.IsBacked {
		t.Error("expected commitment to NOT be backed when API unreachable")
	}
	// Should still report that crons were checked (attempted)
	found := false
	for _, source := range result.CheckedSources {
		if source == "crons" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'crons' in checked sources even on failure, got %v", result.CheckedSources)
	}
}

func TestVerifier_Timeout(t *testing.T) {
	detectedAt := time.Now().Add(-25 * time.Second)

	// Slow server that exceeds timeout
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second)
	}))
	defer server.Close()

	v := NewVerifier(server.URL)
	v.SetTimeout(100 * time.Millisecond)

	result, err := v.Verify(detectedAt)

	// Should degrade gracefully
	if err != nil {
		t.Fatalf("verifier should degrade gracefully on timeout, got error: %v", err)
	}
	if result.IsBacked {
		t.Error("expected commitment to NOT be backed on timeout")
	}
}
