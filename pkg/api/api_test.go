package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/perttulands/oathkeeper/pkg/storage"
)

func testStore(t *testing.T) *storage.Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func insertTestCommitment(t *testing.T, s *storage.Store, id, text, category, status string) {
	t.Helper()
	now := time.Now()
	expires := now.Add(1 * time.Hour)
	c := storage.Commitment{
		ID:         id,
		DetectedAt: now,
		Source:     "test-session",
		MessageID:  "msg-" + id,
		Text:       text,
		Category:   category,
		BackedBy:   []string{},
		Status:     status,
		ExpiresAt:  &expires,
		AlertCount: 0,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.Insert(c); err != nil {
		t.Fatalf("insert commitment: %v", err)
	}
}

func TestNewServer(t *testing.T) {
	s := testStore(t)
	srv := NewServer(s, ":0")
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if srv.store != s {
		t.Error("store not set correctly")
	}
}

func TestNewServerDefaultAddr(t *testing.T) {
	s := testStore(t)
	srv := NewServer(s, "")
	if srv.addr != ":9876" {
		t.Errorf("expected default addr :9876, got %s", srv.addr)
	}
}

func TestListCommitmentsEmpty(t *testing.T) {
	s := testStore(t)
	srv := NewServer(s, ":0")

	req := httptest.NewRequest("GET", "/api/v1/commitments", nil)
	w := httptest.NewRecorder()
	srv.handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp ListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Count != 0 {
		t.Errorf("expected count 0, got %d", resp.Count)
	}
	if len(resp.Commitments) != 0 {
		t.Errorf("expected empty commitments, got %d", len(resp.Commitments))
	}
}

func TestListCommitmentsWithData(t *testing.T) {
	s := testStore(t)
	insertTestCommitment(t, s, "aaa111", "I'll check in 5 minutes", "temporal", "unverified")
	insertTestCommitment(t, s, "bbb222", "I'll notify you", "followup", "backed")

	srv := NewServer(s, ":0")
	req := httptest.NewRequest("GET", "/api/v1/commitments", nil)
	w := httptest.NewRecorder()
	srv.handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp ListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Count != 2 {
		t.Errorf("expected count 2, got %d", resp.Count)
	}
	if len(resp.Commitments) != 2 {
		t.Errorf("expected 2 commitments, got %d", len(resp.Commitments))
	}
}

func TestListCommitmentsFilterByStatus(t *testing.T) {
	s := testStore(t)
	insertTestCommitment(t, s, "aaa111", "I'll check", "temporal", "unverified")
	insertTestCommitment(t, s, "bbb222", "I'll notify", "followup", "backed")

	srv := NewServer(s, ":0")
	req := httptest.NewRequest("GET", "/api/v1/commitments?status=unverified", nil)
	w := httptest.NewRecorder()
	srv.handler().ServeHTTP(w, req)

	var resp ListResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 1 {
		t.Errorf("expected count 1, got %d", resp.Count)
	}
	if resp.Commitments[0].ID != "aaa111" {
		t.Errorf("expected aaa111, got %s", resp.Commitments[0].ID)
	}
}

func TestListCommitmentsFilterByCategory(t *testing.T) {
	s := testStore(t)
	insertTestCommitment(t, s, "aaa111", "I'll check", "temporal", "unverified")
	insertTestCommitment(t, s, "bbb222", "Once done, I'll notify", "conditional", "unverified")

	srv := NewServer(s, ":0")
	req := httptest.NewRequest("GET", "/api/v1/commitments?category=conditional", nil)
	w := httptest.NewRecorder()
	srv.handler().ServeHTTP(w, req)

	var resp ListResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 1 {
		t.Errorf("expected count 1, got %d", resp.Count)
	}
	if resp.Commitments[0].ID != "bbb222" {
		t.Errorf("expected bbb222, got %s", resp.Commitments[0].ID)
	}
}

func TestGetCommitmentByID(t *testing.T) {
	s := testStore(t)
	insertTestCommitment(t, s, "aaa111", "I'll check in 5 minutes", "temporal", "unverified")

	srv := NewServer(s, ":0")
	req := httptest.NewRequest("GET", "/api/v1/commitments/aaa111", nil)
	w := httptest.NewRecorder()
	srv.handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var c storage.Commitment
	if err := json.NewDecoder(w.Body).Decode(&c); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if c.ID != "aaa111" {
		t.Errorf("expected ID aaa111, got %s", c.ID)
	}
	if c.Text != "I'll check in 5 minutes" {
		t.Errorf("expected text 'I'll check in 5 minutes', got %s", c.Text)
	}
}

func TestGetCommitmentNotFound(t *testing.T) {
	s := testStore(t)
	srv := NewServer(s, ":0")

	req := httptest.NewRequest("GET", "/api/v1/commitments/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var errResp ErrorResponse
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Error == "" {
		t.Error("expected error message in response")
	}
}

func TestContentTypeJSON(t *testing.T) {
	s := testStore(t)
	srv := NewServer(s, ":0")

	req := httptest.NewRequest("GET", "/api/v1/commitments", nil)
	w := httptest.NewRecorder()
	srv.handler().ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected content-type application/json, got %s", ct)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	s := testStore(t)
	srv := NewServer(s, ":0")

	req := httptest.NewRequest("POST", "/api/v1/commitments", nil)
	w := httptest.NewRecorder()
	srv.handler().ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestNotFoundRoute(t *testing.T) {
	s := testStore(t)
	srv := NewServer(s, ":0")

	req := httptest.NewRequest("GET", "/api/v1/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHealthEndpoint(t *testing.T) {
	s := testStore(t)
	srv := NewServer(s, ":0")

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	srv.handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected status ok, got %s", resp.Status)
	}
}

func TestListenAndServe(t *testing.T) {
	s := testStore(t)
	srv := NewServer(s, "127.0.0.1:0")

	// Start server in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Shutdown should succeed
	srv.Shutdown()

	// Server should return after shutdown
	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("server did not shut down in time")
	}
}

func TestUnixSocket(t *testing.T) {
	s := testStore(t)
	sockPath := filepath.Join(t.TempDir(), "oathkeeper.sock")
	srv := NewServer(s, "unix:"+sockPath)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Socket file should exist
	if _, err := os.Stat(sockPath); err != nil {
		t.Errorf("socket file should exist: %v", err)
	}

	// Make request via unix socket
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: unixDialer(sockPath),
		},
	}
	resp, err := client.Get("http://unix/api/v1/health")
	if err != nil {
		t.Fatalf("request via unix socket: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var health HealthResponse
	json.NewDecoder(resp.Body).Decode(&health)
	if health.Status != "ok" {
		t.Errorf("expected status ok, got %s", health.Status)
	}

	// Shutdown
	srv.Shutdown()
	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("server did not shut down in time")
	}

	// Socket file should be cleaned up
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Error("socket file should be cleaned up after shutdown")
	}
}

func TestCommitmentJSONFields(t *testing.T) {
	s := testStore(t)
	now := time.Now()
	expires := now.Add(1 * time.Hour)
	c := storage.Commitment{
		ID:         "json-test",
		DetectedAt: now,
		Source:     "test-session",
		MessageID:  "msg-1",
		Text:       "I'll check back",
		Category:   "temporal",
		BackedBy:   []string{"cron:abc123"},
		Status:     "backed",
		ExpiresAt:  &expires,
		AlertCount: 1,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	s.Insert(c)

	srv := NewServer(s, ":0")
	req := httptest.NewRequest("GET", "/api/v1/commitments/json-test", nil)
	w := httptest.NewRecorder()
	srv.handler().ServeHTTP(w, req)

	var raw map[string]interface{}
	json.NewDecoder(w.Body).Decode(&raw)

	// Verify expected JSON fields are present
	for _, field := range []string{"id", "detected_at", "source", "message_id", "text", "category", "backed_by", "status", "expires_at", "alert_count", "created_at", "updated_at"} {
		if _, ok := raw[field]; !ok {
			t.Errorf("missing JSON field: %s", field)
		}
	}

	// Verify backed_by is an array
	backedBy, ok := raw["backed_by"].([]interface{})
	if !ok {
		t.Fatal("backed_by should be an array")
	}
	if len(backedBy) != 1 || backedBy[0] != "cron:abc123" {
		t.Errorf("backed_by should contain cron:abc123, got %v", backedBy)
	}
}

func TestListCommitmentsFilterBySince(t *testing.T) {
	s := testStore(t)
	insertTestCommitment(t, s, "recent", "I'll check", "temporal", "unverified")

	srv := NewServer(s, ":0")
	req := httptest.NewRequest("GET", "/api/v1/commitments?since=1h", nil)
	w := httptest.NewRecorder()
	srv.handler().ServeHTTP(w, req)

	var resp ListResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 1 {
		t.Errorf("expected count 1, got %d", resp.Count)
	}
}

func TestListCommitmentsInvalidSince(t *testing.T) {
	s := testStore(t)
	srv := NewServer(s, ":0")

	req := httptest.NewRequest("GET", "/api/v1/commitments?since=invalid", nil)
	w := httptest.NewRecorder()
	srv.handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
