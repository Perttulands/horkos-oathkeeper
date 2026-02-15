package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/perttulands/oathkeeper/pkg/beads"
	"github.com/perttulands/oathkeeper/pkg/detector"
	"github.com/perttulands/oathkeeper/pkg/grace"
)

type analyzeResponse struct {
	Commitment bool     `json:"commitment"`
	Category   string   `json:"category"`
	Confidence float64  `json:"confidence"`
	Text       string   `json:"text"`
	Resolved   []string `json:"resolved"`
}

type commitmentResponse struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	Tags        []string  `json:"tags"`
	CreatedAt   time.Time `json:"created_at"`
	ClosedAt    time.Time `json:"closed_at,omitempty"`
	CloseReason string    `json:"close_reason,omitempty"`
}

type resolveResponse struct {
	ID       string `json:"id"`
	Resolved bool   `json:"resolved"`
}

type statsResponse struct {
	Total      int            `json:"total"`
	Open       int            `json:"open"`
	Resolved   int            `json:"resolved"`
	ByCategory map[string]int `json:"by_category"`
}

func TestV2AnalyzeCommitmentStartsGraceAndReturnsCommitment(t *testing.T) {
	var scheduleCalls int
	var autoResolveCalls int
	var scheduledAt time.Time

	v2 := &V2API{
		detectCommitment: func(message string) detector.DetectionResult {
			return detector.DetectionResult{
				IsCommitment:   true,
				Category:       detector.CategoryTemporal,
				CommitmentText: "I'll check on that in 10 minutes",
				Confidence:     0.95,
			}
		},
		autoResolve: func(sessionKey, message string) ([]string, error) {
			autoResolveCalls++
			return []string{}, nil
		},
		scheduleGrace: func(commitmentID string, detectedAt time.Time, callback func(grace.VerificationOutcome)) {
			scheduleCalls++
			scheduledAt = detectedAt
		},
		now: func() time.Time {
			return time.Date(2026, 2, 14, 20, 0, 0, 0, time.UTC)
		},
	}

	reqBody := []byte(`{"session_key":"main","message":"I'll check on that in 10 minutes","role":"assistant"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/analyze", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp analyzeResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !resp.Commitment {
		t.Fatalf("expected commitment=true, got false")
	}
	if resp.Category != "temporal" {
		t.Fatalf("expected category temporal, got %q", resp.Category)
	}
	if resp.Confidence != 0.95 {
		t.Fatalf("expected confidence 0.95, got %v", resp.Confidence)
	}
	if resp.Text != "I'll check on that in 10 minutes" {
		t.Fatalf("unexpected text: %q", resp.Text)
	}
	if scheduleCalls != 1 {
		t.Fatalf("expected schedule call once, got %d", scheduleCalls)
	}
	if !scheduledAt.Equal(time.Date(2026, 2, 14, 20, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected scheduledAt: %v", scheduledAt)
	}
	if autoResolveCalls != 0 {
		t.Fatalf("expected auto resolve not to be called, got %d", autoResolveCalls)
	}
}

func TestV2AnalyzeNonCommitmentReturnsEmptyResolved(t *testing.T) {
	var scheduleCalls int
	var autoResolveCalls int

	v2 := &V2API{
		detectCommitment: func(message string) detector.DetectionResult {
			return detector.DetectionResult{IsCommitment: false}
		},
		autoResolve: func(sessionKey, message string) ([]string, error) {
			autoResolveCalls++
			return []string{}, nil
		},
		scheduleGrace: func(commitmentID string, detectedAt time.Time, callback func(grace.VerificationOutcome)) {
			scheduleCalls++
		},
		now: time.Now,
	}

	reqBody := []byte(`{"session_key":"main","message":"This is just a status update","role":"assistant"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/analyze", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp analyzeResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Commitment {
		t.Fatalf("expected commitment=false, got true")
	}
	if len(resp.Resolved) != 0 {
		t.Fatalf("expected empty resolved list, got %v", resp.Resolved)
	}
	if autoResolveCalls != 1 {
		t.Fatalf("expected autoResolve call once, got %d", autoResolveCalls)
	}
	if scheduleCalls != 0 {
		t.Fatalf("expected schedule not called, got %d", scheduleCalls)
	}
}

func TestV2AnalyzeNonCommitmentReturnsResolvedBeads(t *testing.T) {
	v2 := &V2API{
		detectCommitment: func(message string) detector.DetectionResult {
			return detector.DetectionResult{IsCommitment: false}
		},
		autoResolve: func(sessionKey, message string) ([]string, error) {
			return []string{"bd-123"}, nil
		},
		now: time.Now,
	}

	reqBody := []byte(`{"session_key":"main","message":"I checked that and here are the results","role":"assistant"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/analyze", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp analyzeResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Commitment {
		t.Fatalf("expected commitment=false, got true")
	}
	if len(resp.Resolved) != 1 || resp.Resolved[0] != "bd-123" {
		t.Fatalf("expected resolved [bd-123], got %v", resp.Resolved)
	}
}

func TestV2AnalyzeIgnoresNonAssistantMessages(t *testing.T) {
	var detectCalls int
	var autoResolveCalls int

	v2 := &V2API{
		detectCommitment: func(message string) detector.DetectionResult {
			detectCalls++
			return detector.DetectionResult{IsCommitment: true}
		},
		autoResolve: func(sessionKey, message string) ([]string, error) {
			autoResolveCalls++
			return []string{"bd-123"}, nil
		},
		now: time.Now,
	}

	reqBody := []byte(`{"session_key":"main","message":"I'll check that","role":"user"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/analyze", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp analyzeResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Commitment {
		t.Fatalf("expected commitment=false for non-assistant role")
	}
	if len(resp.Resolved) != 0 {
		t.Fatalf("expected empty resolved list, got %v", resp.Resolved)
	}
	if detectCalls != 0 {
		t.Fatalf("expected detector not called, got %d", detectCalls)
	}
	if autoResolveCalls != 0 {
		t.Fatalf("expected autoResolve not called, got %d", autoResolveCalls)
	}
}

func TestV2AnalyzeInvalidJSON(t *testing.T) {
	v2 := NewV2API(nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/analyze", bytes.NewReader([]byte("{")))
	w := httptest.NewRecorder()

	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestV2CommitmentsListOpenCommitments(t *testing.T) {
	var gotFilter beads.Filter

	v2 := &V2API{
		listBeads: func(filter beads.Filter) ([]beads.Bead, error) {
			gotFilter = filter
			return []beads.Bead{
				{ID: "bd-1", Title: "oathkeeper: check logs", Status: "open", Tags: []string{"oathkeeper", "temporal"}},
			}, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/commitments?status=open", nil)
	w := httptest.NewRecorder()

	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotFilter.Status != "open" {
		t.Fatalf("expected status filter open, got %q", gotFilter.Status)
	}

	var resp []commitmentResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 commitment, got %d", len(resp))
	}
	if resp[0].ID != "bd-1" {
		t.Fatalf("expected id bd-1, got %q", resp[0].ID)
	}
}

func TestV2CommitmentsListFiltersByCategory(t *testing.T) {
	var gotFilter beads.Filter

	v2 := &V2API{
		listBeads: func(filter beads.Filter) ([]beads.Bead, error) {
			gotFilter = filter
			return []beads.Bead{}, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/commitments?status=open&category=temporal", nil)
	w := httptest.NewRecorder()

	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotFilter.Status != "open" {
		t.Fatalf("expected status filter open, got %q", gotFilter.Status)
	}
	if gotFilter.Category != "temporal" {
		t.Fatalf("expected category filter temporal, got %q", gotFilter.Category)
	}
}

func TestV2CommitmentResolveViaAPI(t *testing.T) {
	var resolvedID string
	var resolvedReason string

	v2 := &V2API{
		resolveBead: func(beadID, reason string) error {
			resolvedID = beadID
			resolvedReason = reason
			return nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/commitments/bd-123/resolve", bytes.NewReader([]byte(`{"reason":"verified manually"}`)))
	w := httptest.NewRecorder()

	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if resolvedID != "bd-123" {
		t.Fatalf("expected resolved id bd-123, got %q", resolvedID)
	}
	if resolvedReason != "verified manually" {
		t.Fatalf("expected resolve reason propagated, got %q", resolvedReason)
	}

	var resp resolveResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Resolved {
		t.Fatalf("expected resolved=true")
	}
	if resp.ID != "bd-123" {
		t.Fatalf("expected response id bd-123, got %q", resp.ID)
	}
}

func TestV2CommitmentByIDUnknownReturns404(t *testing.T) {
	v2 := &V2API{
		getBead: func(beadID string) (beads.Bead, error) {
			return beads.Bead{}, beads.ErrBeadNotFound
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/commitments/missing-id", nil)
	w := httptest.NewRecorder()

	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestV2StatsMixedStates(t *testing.T) {
	var gotFilter beads.Filter

	v2 := &V2API{
		listBeads: func(filter beads.Filter) ([]beads.Bead, error) {
			gotFilter = filter
			return []beads.Bead{
				{ID: "bd-1", Status: "open", Tags: []string{"oathkeeper", "temporal"}},
				{ID: "bd-2", Status: "closed", Tags: []string{"oathkeeper", "temporal"}},
				{ID: "bd-3", Status: "closed", Tags: []string{"oathkeeper", "conditional"}},
			}, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/stats", nil)
	w := httptest.NewRecorder()

	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if gotFilter != (beads.Filter{}) {
		t.Fatalf("expected empty filter for stats, got %+v", gotFilter)
	}

	var resp statsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Total != 3 {
		t.Fatalf("expected total=3, got %d", resp.Total)
	}
	if resp.Open != 1 {
		t.Fatalf("expected open=1, got %d", resp.Open)
	}
	if resp.Resolved != 2 {
		t.Fatalf("expected resolved=2, got %d", resp.Resolved)
	}
	if resp.ByCategory["temporal"] != 2 {
		t.Fatalf("expected temporal=2, got %d", resp.ByCategory["temporal"])
	}
	if resp.ByCategory["conditional"] != 1 {
		t.Fatalf("expected conditional=1, got %d", resp.ByCategory["conditional"])
	}
}

func TestV2StatsEmpty(t *testing.T) {
	v2 := &V2API{
		listBeads: func(filter beads.Filter) ([]beads.Bead, error) {
			return []beads.Bead{}, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/stats", nil)
	w := httptest.NewRecorder()

	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp statsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Total != 0 || resp.Open != 0 || resp.Resolved != 0 {
		t.Fatalf("expected zero stats, got %+v", resp)
	}
	if len(resp.ByCategory) != 0 {
		t.Fatalf("expected empty by_category, got %+v", resp.ByCategory)
	}
}

func TestV2GraceCallbackFiredOnCommitment(t *testing.T) {
	// Verify that when a commitment is detected and grace period fires,
	// the graceCallback receives the commitment info.
	var mu sync.Mutex
	var callbackCalls int
	var gotMessage string
	var gotCategory string
	var gotCommitmentID string

	v2 := &V2API{
		detectCommitment: func(message string) detector.DetectionResult {
			return detector.DetectionResult{
				IsCommitment:   true,
				Category:       detector.CategoryFollowup,
				CommitmentText: "I'll monitor the build",
				Confidence:     0.90,
			}
		},
		// Schedule the grace period and immediately fire the callback
		scheduleGrace: func(commitmentID string, detectedAt time.Time, callback func(grace.VerificationOutcome)) {
			// Simulate grace period completing immediately
			callback(grace.VerificationOutcome{
				CommitmentID: commitmentID,
				IsBacked:     false,
				Mechanisms:   []string{},
			})
		},
		graceCallback: func(commitmentID string, message string, category string, outcome grace.VerificationOutcome) {
			mu.Lock()
			defer mu.Unlock()
			callbackCalls++
			gotCommitmentID = commitmentID
			gotMessage = message
			gotCategory = category
		},
		now: func() time.Time {
			return time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
		},
	}

	reqBody := []byte(`{"session_key":"test-session","message":"I'll monitor the build","role":"assistant"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/analyze", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	mu.Lock()
	defer mu.Unlock()
	if callbackCalls != 1 {
		t.Fatalf("expected grace callback called once, got %d", callbackCalls)
	}
	if gotMessage != "I'll monitor the build" {
		t.Fatalf("expected message 'I'll monitor the build', got %q", gotMessage)
	}
	if gotCategory != "followup" {
		t.Fatalf("expected category 'followup', got %q", gotCategory)
	}
	if gotCommitmentID == "" {
		t.Fatal("expected non-empty commitment ID")
	}
}

func TestV2GraceCallbackNotFiredWhenBacked(t *testing.T) {
	// When the commitment IS backed, the callback should still fire
	// (the callback itself decides what to do based on IsBacked).
	var callbackCalls int

	v2 := &V2API{
		detectCommitment: func(message string) detector.DetectionResult {
			return detector.DetectionResult{
				IsCommitment:   true,
				Category:       detector.CategoryTemporal,
				CommitmentText: "I'll check in 5 minutes",
				Confidence:     0.95,
			}
		},
		scheduleGrace: func(commitmentID string, detectedAt time.Time, callback func(grace.VerificationOutcome)) {
			callback(grace.VerificationOutcome{
				CommitmentID: commitmentID,
				IsBacked:     true,
				Mechanisms:   []string{"cron:abc123"},
			})
		},
		graceCallback: func(commitmentID string, message string, category string, outcome grace.VerificationOutcome) {
			callbackCalls++
			if !outcome.IsBacked {
				t.Error("expected outcome to be backed")
			}
		},
		now: time.Now,
	}

	reqBody := []byte(`{"session_key":"test","message":"I'll check in 5 minutes","role":"assistant"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/analyze", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if callbackCalls != 1 {
		t.Fatalf("expected callback called once, got %d", callbackCalls)
	}
}

func TestV2SetGraceCallback(t *testing.T) {
	v2 := NewV2API(nil, nil, nil)

	called := false
	v2.SetGraceCallback(func(commitmentID string, message string, category string, outcome grace.VerificationOutcome) {
		called = true
	})

	if v2.graceCallback == nil {
		t.Fatal("expected graceCallback to be set")
	}

	// Invoke it to verify it works
	v2.graceCallback("test", "msg", "cat", grace.VerificationOutcome{})
	if !called {
		t.Fatal("expected callback to be called")
	}
}

func TestV2AnalyzeMethodNotAllowed(t *testing.T) {
	v2 := NewV2API(nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/analyze", nil)
	w := httptest.NewRecorder()

	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestV2CommitmentResolveEmptyReason(t *testing.T) {
	v2 := &V2API{
		resolveBead: func(beadID, reason string) error {
			return nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/commitments/bd-123/resolve", bytes.NewReader([]byte(`{"reason":""}`)))
	w := httptest.NewRecorder()

	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty reason, got %d", w.Code)
	}
}
