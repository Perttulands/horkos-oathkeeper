package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/perttulands/horkos-oathkeeper/pkg/beads"
	"github.com/perttulands/horkos-oathkeeper/pkg/detector"
	"github.com/perttulands/horkos-oathkeeper/pkg/grace"
)

type analyzeResponse struct {
	Commitment bool                `json:"commitment"`
	Category   string              `json:"category"`
	Confidence float64             `json:"confidence"`
	Text       string              `json:"text"`
	Resolved   []string            `json:"resolved"`
	Fulfilled  []FulfilledResponse `json:"fulfilled"`
	Escalated  []EscalatedResponse `json:"escalated"`
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
			return []string{"br-123"}, nil
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
	if len(resp.Resolved) != 1 || resp.Resolved[0] != "br-123" {
		t.Fatalf("expected resolved [br-123], got %v", resp.Resolved)
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
			return []string{"br-123"}, nil
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
				{ID: "br-1", Title: "oathkeeper: check logs", Status: "open", Tags: []string{"oathkeeper", "temporal"}},
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
	if resp[0].ID != "br-1" {
		t.Fatalf("expected id br-1, got %q", resp[0].ID)
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

	req := httptest.NewRequest(http.MethodPost, "/api/v2/commitments/br-123/resolve", bytes.NewReader([]byte(`{"reason":"verified manually"}`)))
	w := httptest.NewRecorder()

	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if resolvedID != "br-123" {
		t.Fatalf("expected resolved id br-123, got %q", resolvedID)
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
	if resp.ID != "br-123" {
		t.Fatalf("expected response id br-123, got %q", resp.ID)
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

func TestV2CommitmentsListReturns503WhenWorkspaceNotInitialized(t *testing.T) {
	v2 := &V2API{
		listBeads: func(filter beads.Filter) ([]beads.Bead, error) {
			return nil, errors.New("Beads not initialized: run 'br init'")
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/commitments", nil)
	w := httptest.NewRecorder()
	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if resp.Hint == "" {
		t.Fatal("expected hint for not initialized backend")
	}
}

func TestV2CommitmentByIDReturns504OnTimeout(t *testing.T) {
	v2 := &V2API{
		getBead: func(beadID string) (beads.Bead, error) {
			return beads.Bead{}, context.DeadlineExceeded
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/commitments/br-123", nil)
	w := httptest.NewRecorder()
	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d", w.Code)
	}
}

func TestV2ResolveReturns503WhenCommandUnavailable(t *testing.T) {
	v2 := &V2API{
		resolveBead: func(beadID, reason string) error {
			return fmt.Errorf("%w: br", beads.ErrCommandUnavailable)
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/commitments/br-123/resolve", bytes.NewReader([]byte(`{"reason":"done"}`)))
	w := httptest.NewRecorder()
	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestV2StatsMixedStates(t *testing.T) {
	var gotFilter beads.Filter

	v2 := &V2API{
		listBeads: func(filter beads.Filter) ([]beads.Bead, error) {
			gotFilter = filter
			return []beads.Bead{
				{ID: "br-1", Status: "open", Tags: []string{"oathkeeper", "temporal"}},
				{ID: "br-2", Status: "closed", Tags: []string{"oathkeeper", "temporal"}},
				{ID: "br-3", Status: "closed", Tags: []string{"oathkeeper", "conditional"}},
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
	var gotSessionKey string
	var gotDetectedAt time.Time

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
		graceCallback: func(meta GraceCallbackContext, outcome grace.VerificationOutcome) {
			mu.Lock()
			defer mu.Unlock()
			callbackCalls++
			gotCommitmentID = meta.CommitmentID
			gotMessage = meta.Message
			gotCategory = meta.Category
			gotSessionKey = meta.SessionKey
			gotDetectedAt = meta.DetectedAt
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
	if gotSessionKey != "test-session" {
		t.Fatalf("expected session key test-session, got %q", gotSessionKey)
	}
	if !gotDetectedAt.Equal(time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("expected detectedAt to match mocked clock, got %v", gotDetectedAt)
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
		graceCallback: func(meta GraceCallbackContext, outcome grace.VerificationOutcome) {
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
	v2.SetGraceCallback(func(meta GraceCallbackContext, outcome grace.VerificationOutcome) {
		called = true
	})

	if v2.graceCallback == nil {
		t.Fatal("expected graceCallback to be set")
	}

	// Invoke it to verify it works
	v2.graceCallback(GraceCallbackContext{
		CommitmentID: "test",
		Message:      "msg",
		Category:     "cat",
	}, grace.VerificationOutcome{})
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

func TestV2ManualResolveTriggerOnResolveCallback(t *testing.T) {
	var mu sync.Mutex
	var gotBeadID, gotEvidence string
	var callCount int
	done := make(chan struct{}, 1)

	v2 := &V2API{
		resolveBead: func(beadID, reason string) error {
			return nil
		},
	}
	v2.SetResolveCallback(func(beadID, evidence string) {
		mu.Lock()
		defer mu.Unlock()
		callCount++
		gotBeadID = beadID
		gotEvidence = evidence
		done <- struct{}{}
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v2/commitments/br-555/resolve", bytes.NewReader([]byte(`{"reason":"verified manually"}`)))
	w := httptest.NewRecorder()

	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for onResolve callback")
	}

	mu.Lock()
	defer mu.Unlock()
	if callCount != 1 {
		t.Fatalf("expected onResolve called once, got %d", callCount)
	}
	if gotBeadID != "br-555" {
		t.Fatalf("expected beadID br-555, got %q", gotBeadID)
	}
	if gotEvidence != "verified manually" {
		t.Fatalf("expected evidence 'verified manually', got %q", gotEvidence)
	}
}

func TestV2AutoResolveTriggerOnResolveCallback(t *testing.T) {
	var mu sync.Mutex
	var gotIDs []string
	done := make(chan struct{}, 2)

	v2 := &V2API{
		detectCommitment: func(message string) detector.DetectionResult {
			return detector.DetectionResult{IsCommitment: false}
		},
		autoResolve: func(sessionKey, message string) ([]string, error) {
			return []string{"br-aaa", "br-bbb"}, nil
		},
		now: time.Now,
	}
	v2.SetResolveCallback(func(beadID, evidence string) {
		mu.Lock()
		defer mu.Unlock()
		gotIDs = append(gotIDs, beadID)
		done <- struct{}{}
	})

	reqBody := []byte(`{"session_key":"main","message":"I checked and fixed everything","role":"assistant"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/analyze", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Wait for both callbacks
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for onResolve callbacks")
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(gotIDs) != 2 {
		t.Fatalf("expected onResolve called twice, got %d: %v", len(gotIDs), gotIDs)
	}
	found := map[string]bool{}
	for _, id := range gotIDs {
		found[id] = true
	}
	if !found["br-aaa"] || !found["br-bbb"] {
		t.Fatalf("expected br-aaa and br-bbb, got %v", gotIDs)
	}
}

func TestV2OnResolveNilDoesNotPanic(t *testing.T) {
	// V2API with no resolve callback set — manual resolve should not panic
	v2 := &V2API{
		resolveBead: func(beadID, reason string) error {
			return nil
		},
	}
	// Explicitly don't set resolve callback

	req := httptest.NewRequest(http.MethodPost, "/api/v2/commitments/br-999/resolve", bytes.NewReader([]byte(`{"reason":"test"}`)))
	w := httptest.NewRecorder()

	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Also test auto-resolve path with nil callback
	v2auto := &V2API{
		detectCommitment: func(message string) detector.DetectionResult {
			return detector.DetectionResult{IsCommitment: false}
		},
		autoResolve: func(sessionKey, message string) ([]string, error) {
			return []string{"br-resolved"}, nil
		},
		now: time.Now,
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v2/analyze", bytes.NewReader([]byte(`{"session_key":"s","message":"done","role":"assistant"}`)))
	w2 := httptest.NewRecorder()

	v2auto.Handler().ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}
}

func TestV2CommitmentResolveEmptyReason(t *testing.T) {
	v2 := &V2API{
		resolveBead: func(beadID, reason string) error {
			return nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/commitments/br-123/resolve", bytes.NewReader([]byte(`{"reason":""}`)))
	w := httptest.NewRecorder()

	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty reason, got %d", w.Code)
	}
}

// --- US-008 Context-aware auto-resolution tests ---

func newContextV2(opts ...func(*V2API)) *V2API {
	v2 := &V2API{
		detectCommitment: func(message string) detector.DetectionResult {
			// Use real detector for context tests
			d := detector.NewDetector()
			return d.DetectCommitment(message)
		},
		autoResolve: func(sessionKey, message string) ([]string, error) {
			return []string{}, nil
		},
		now: time.Now,
	}
	ca := detector.NewContextAnalyzer(5)
	v2.SetContextAnalyzer(ca, 5)
	for _, opt := range opts {
		opt(v2)
	}
	return v2
}

func postAnalyze(t *testing.T, handler http.Handler, sessionKey, message string) analyzeResponse {
	t.Helper()
	body := fmt.Sprintf(`{"session_key":%q,"message":%q,"role":"assistant"}`, sessionKey, message)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/analyze", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp analyzeResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

func TestV2ContextFulfillmentAcrossMessages(t *testing.T) {
	// POST 3 messages: commitment → unrelated → fulfillment
	// Verify fulfilled in response.
	// Note: the ContextAnalyzer has its own internal Detector, so fulfillment
	// detection works even when the top-level detector doesn't flag "I need to check..."
	// as a commitment (different pattern sets). The context analyzer sees the
	// commitment pattern "I need to" in its own pass.
	v2 := newContextV2()
	h := v2.Handler()

	// Message 1: commitment (uses "I need to" which context analyzer detects)
	postAnalyze(t, h, "sess-1", "I need to check the logs for errors")

	// Message 2: unrelated
	postAnalyze(t, h, "sess-1", "Looking at the deployment now")

	// Message 3: fulfillment
	resp3 := postAnalyze(t, h, "sess-1", "I checked the logs and found the issue")
	if len(resp3.Fulfilled) == 0 {
		t.Fatalf("expected fulfilled commitments in response, got none")
	}
	found := false
	for _, f := range resp3.Fulfilled {
		if strings.Contains(f.FulfilledBy, "checked the logs") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected fulfillment mentioning 'checked the logs', got %+v", resp3.Fulfilled)
	}
}

func TestV2ContextEscalationOnRepeatedCommitments(t *testing.T) {
	// POST 2 commitments of same type, verify escalated in response
	v2 := newContextV2()
	h := v2.Handler()

	// Two followup-type commitments
	postAnalyze(t, h, "sess-esc", "I'll monitor the build output")
	resp := postAnalyze(t, h, "sess-esc", "I'll watch the deployment logs")

	if len(resp.Escalated) == 0 {
		t.Fatalf("expected escalated commitments, got none")
	}
	if resp.Escalated[0].Count < 2 {
		t.Fatalf("expected escalation count >= 2, got %d", resp.Escalated[0].Count)
	}
}

func TestV2ContextSessionIsolation(t *testing.T) {
	// Different sessions don't cross-contaminate
	v2 := newContextV2()
	h := v2.Handler()

	// Session A: commitment
	postAnalyze(t, h, "sess-a", "I'll check the logs for errors")

	// Session B: fulfillment text — should NOT match session A's commitment
	resp := postAnalyze(t, h, "sess-b", "I checked the logs and found the issue")

	// Session B has no prior commitment, so fulfillment shouldn't appear
	if len(resp.Fulfilled) != 0 {
		t.Fatalf("expected no fulfilled (different session), got %+v", resp.Fulfilled)
	}
}

func TestV2ContextBufferTrimming(t *testing.T) {
	// Buffer trimming beyond window size
	v2 := newContextV2()
	// Set a small window
	v2.SetContextAnalyzer(detector.NewContextAnalyzer(2), 2)
	h := v2.Handler()

	// Message 1: commitment (will be trimmed out of window)
	postAnalyze(t, h, "sess-trim", "I'll check the logs for errors")

	// Message 2: filler
	postAnalyze(t, h, "sess-trim", "Working on something else entirely")

	// Message 3: fulfillment (commitment is now outside window of 2)
	resp := postAnalyze(t, h, "sess-trim", "I checked the logs and everything is fine")

	// With window=2, only last 2 messages are in buffer, so the commitment
	// from message 1 should be outside the window
	if len(resp.Fulfilled) != 0 {
		t.Fatalf("expected no fulfilled (commitment outside window), got %+v", resp.Fulfilled)
	}
}

func TestV2ContextConcurrentSessions(t *testing.T) {
	// 3 goroutines posting to same session, no race
	v2 := newContextV2()
	h := v2.Handler()

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			msg := fmt.Sprintf("Message number %d from goroutine", idx)
			body := fmt.Sprintf(`{"session_key":"concurrent","message":%q,"role":"assistant"}`, msg)
			req := httptest.NewRequest(http.MethodPost, "/api/v2/analyze", bytes.NewReader([]byte(body)))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Errorf("goroutine %d: expected 200, got %d", idx, w.Code)
			}
		}(i)
	}
	wg.Wait()

	// Verify buffer has messages (check via another request)
	v2.mu.Lock()
	sb := v2.sessions["concurrent"]
	v2.mu.Unlock()
	if sb == nil || len(sb.messages) == 0 {
		t.Fatal("expected session buffer to have messages after concurrent writes")
	}
}

func TestV2ContextNonAssistantDoesNotAffectBuffer(t *testing.T) {
	v2 := newContextV2()
	h := v2.Handler()

	// Post as user (non-assistant) — should not be buffered
	body := `{"session_key":"sess-na","message":"I'll check the logs","role":"user"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/analyze", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Buffer should be empty for this session
	v2.mu.Lock()
	sb := v2.sessions["sess-na"]
	v2.mu.Unlock()
	if sb != nil && len(sb.messages) > 0 {
		t.Fatalf("expected empty buffer for non-assistant role, got %d messages", len(sb.messages))
	}
}

// --- Tests for previously uncovered paths ---

func TestMatchesSession(t *testing.T) {
	tests := []struct {
		name       string
		tags       []string
		sessionKey string
		want       bool
	}{
		{
			name:       "empty session key matches anything",
			tags:       []string{"oathkeeper", "session-main"},
			sessionKey: "",
			want:       true,
		},
		{
			name:       "matching session tag",
			tags:       []string{"oathkeeper", "session-main"},
			sessionKey: "main",
			want:       true,
		},
		{
			name:       "non-matching session tag",
			tags:       []string{"oathkeeper", "session-other"},
			sessionKey: "main",
			want:       false,
		},
		{
			name:       "case insensitive match",
			tags:       []string{"oathkeeper", "Session-Main"},
			sessionKey: "MAIN",
			want:       true,
		},
		{
			name:       "no session tag on bead matches any session",
			tags:       []string{"oathkeeper", "temporal"},
			sessionKey: "main",
			want:       true,
		},
		{
			name:       "empty tags with session key",
			tags:       []string{},
			sessionKey: "main",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesSession(tt.tags, tt.sessionKey)
			if got != tt.want {
				t.Fatalf("matchesSession(%v, %q) = %v, want %v", tt.tags, tt.sessionKey, got, tt.want)
			}
		})
	}
}

func TestV2AddContextResultsClosesBeadForFulfilled(t *testing.T) {
	// When context analyzer detects fulfillment and there are open beads
	// matching the session, addContextResults should resolve them.
	var resolvedIDs []string
	var resolveReasons []string

	v2 := newContextV2(func(v *V2API) {
		v.listBeads = func(filter beads.Filter) ([]beads.Bead, error) {
			return []beads.Bead{
				{ID: "br-1", Status: "open", Tags: []string{"oathkeeper", "session-test-sess"}},
				{ID: "br-2", Status: "open", Tags: []string{"oathkeeper", "session-other"}},
			}, nil
		}
		v.resolveBead = func(beadID, reason string) error {
			resolvedIDs = append(resolvedIDs, beadID)
			resolveReasons = append(resolveReasons, reason)
			return nil
		}
	})
	h := v2.Handler()

	// Message 1: commitment
	postAnalyze(t, h, "test-sess", "I need to check the deployment logs")

	// Message 2: fulfillment
	postAnalyze(t, h, "test-sess", "I checked the logs and found the error")

	// Verify that the matching bead was resolved
	if len(resolvedIDs) == 0 {
		t.Fatal("expected at least one bead to be resolved via context fulfillment")
	}
	if resolvedIDs[0] != "br-1" {
		t.Fatalf("expected br-1 to be resolved, got %v", resolvedIDs)
	}
	// br-2 should not be resolved (different session)
	for _, id := range resolvedIDs {
		if id == "br-2" {
			t.Fatal("br-2 should not have been resolved (different session)")
		}
	}
	if len(resolveReasons) > 0 && !strings.Contains(resolveReasons[0], "fulfilled") {
		t.Fatalf("expected reason to contain 'fulfilled', got %q", resolveReasons[0])
	}
}

func TestV2AddContextResultsNilContextAnalyzer(t *testing.T) {
	// When contextAnalyzer is nil, addContextResults should be a no-op
	v2 := &V2API{
		detectCommitment: func(message string) detector.DetectionResult {
			return detector.DetectionResult{IsCommitment: false}
		},
		autoResolve: func(sessionKey, message string) ([]string, error) {
			return []string{}, nil
		},
		now: time.Now,
	}
	// contextAnalyzer is nil, sessions is nil

	resp := AnalyzeResponse{Commitment: false}
	// Should not panic
	v2.addContextResults(&resp, []string{"some message"}, "sess")
	if len(resp.Fulfilled) != 0 {
		t.Fatalf("expected no fulfilled with nil analyzer, got %+v", resp.Fulfilled)
	}
}

func TestV2StatsErrorFromListBeads(t *testing.T) {
	v2 := &V2API{
		listBeads: func(filter beads.Filter) ([]beads.Bead, error) {
			return nil, fmt.Errorf("br list timed out: %w", context.DeadlineExceeded)
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/stats", nil)
	w := httptest.NewRecorder()
	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504 for timeout error, got %d", w.Code)
	}
}

func TestV2StatsMethodNotAllowed(t *testing.T) {
	v2 := &V2API{
		listBeads: func(filter beads.Filter) ([]beads.Bead, error) {
			return nil, nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/stats", nil)
	w := httptest.NewRecorder()
	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestV2StatsNilListBeads(t *testing.T) {
	v2 := &V2API{}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/stats", nil)
	w := httptest.NewRecorder()
	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for nil listBeads, got %d", w.Code)
	}
}

func TestV2CommitmentsNilListBeads(t *testing.T) {
	v2 := &V2API{}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/commitments", nil)
	w := httptest.NewRecorder()
	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for nil listBeads, got %d", w.Code)
	}
}

func TestV2CommitmentByIDNilGetBead(t *testing.T) {
	v2 := &V2API{}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/commitments/br-123", nil)
	w := httptest.NewRecorder()
	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for nil getBead, got %d", w.Code)
	}
}

func TestV2CommitmentByIDEmptyPath(t *testing.T) {
	v2 := &V2API{}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/commitments/", nil)
	w := httptest.NewRecorder()
	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty path, got %d", w.Code)
	}
}

func TestV2CommitmentByIDSlashInPath(t *testing.T) {
	v2 := &V2API{
		getBead: func(beadID string) (beads.Bead, error) {
			return beads.Bead{}, nil
		},
	}

	// Path with nested slash (not /resolve) should return 404
	req := httptest.NewRequest(http.MethodGet, "/api/v2/commitments/br-123/unknown", nil)
	w := httptest.NewRecorder()
	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nested path, got %d", w.Code)
	}
}

func TestV2CommitmentByIDMethodNotAllowed(t *testing.T) {
	v2 := &V2API{
		getBead: func(beadID string) (beads.Bead, error) {
			return beads.Bead{}, nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/commitments/br-123", nil)
	w := httptest.NewRecorder()
	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestV2ResolveNilResolveBead(t *testing.T) {
	v2 := &V2API{}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/commitments/br-123/resolve",
		bytes.NewReader([]byte(`{"reason":"done"}`)))
	w := httptest.NewRecorder()
	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for nil resolveBead, got %d", w.Code)
	}
}

func TestV2ResolveMethodNotAllowed(t *testing.T) {
	v2 := &V2API{
		resolveBead: func(beadID, reason string) error { return nil },
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/commitments/br-123/resolve", nil)
	w := httptest.NewRecorder()
	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestBeadCategory(t *testing.T) {
	tests := []struct {
		name string
		tags []string
		want string
	}{
		{"normal category", []string{"oathkeeper", "temporal"}, "temporal"},
		{"skip oathkeeper only", []string{"oathkeeper"}, ""},
		{"skip session tag", []string{"oathkeeper", "session-abc", "followup"}, "followup"},
		{"empty tags", nil, ""},
		{"first non-system tag wins", []string{"oathkeeper", "session-x", "conditional", "temporal"}, "conditional"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := beadCategory(tt.tags)
			if got != tt.want {
				t.Fatalf("beadCategory(%v) = %q, want %q", tt.tags, got, tt.want)
			}
		})
	}
}

func TestFormatAnalyzeCommitmentID(t *testing.T) {
	ts := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	// Normal session key
	id := formatAnalyzeCommitmentID("main", ts)
	if !strings.HasPrefix(id, "v2-main-") {
		t.Fatalf("expected v2-main-* prefix, got %q", id)
	}

	// Empty session key defaults to "default"
	id = formatAnalyzeCommitmentID("", ts)
	if !strings.HasPrefix(id, "v2-default-") {
		t.Fatalf("expected v2-default-* prefix, got %q", id)
	}

	// Special characters replaced
	id = formatAnalyzeCommitmentID("my session@123", ts)
	if strings.Contains(id, " ") || strings.Contains(id, "@") {
		t.Fatalf("expected special chars replaced, got %q", id)
	}
}

func TestV2CommitmentsListErrorPath(t *testing.T) {
	v2 := &V2API{
		listBeads: func(filter beads.Filter) ([]beads.Bead, error) {
			return nil, errors.New("beads command unavailable: br")
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/commitments", nil)
	w := httptest.NewRecorder()
	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for command unavailable, got %d", w.Code)
	}
}

func TestV2AutoResolveErrorPath(t *testing.T) {
	v2 := &V2API{
		detectCommitment: func(message string) detector.DetectionResult {
			return detector.DetectionResult{IsCommitment: false}
		},
		autoResolve: func(sessionKey, message string) ([]string, error) {
			return nil, context.DeadlineExceeded
		},
		now: time.Now,
	}

	reqBody := []byte(`{"session_key":"main","message":"I checked that","role":"assistant"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/analyze", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()
	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504 for auto-resolve timeout, got %d", w.Code)
	}
}

func TestV2SetResolveBead(t *testing.T) {
	v2 := NewV2API(nil, nil, nil)
	called := false
	v2.SetResolveBead(func(beadID, reason string) error {
		called = true
		return nil
	})

	// Verify it's set
	if v2.resolveBead == nil {
		t.Fatal("expected resolveBead to be set")
	}

	// Verify it works by calling through resolve endpoint
	req := httptest.NewRequest(http.MethodPost, "/api/v2/commitments/test/resolve",
		bytes.NewReader([]byte(`{"reason":"done"}`)))
	w := httptest.NewRecorder()
	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !called {
		t.Fatal("expected resolveBead to be called")
	}
}

func TestNewV2APIWithFuncs(t *testing.T) {
	detectCalled := false
	v2 := NewV2APIWithFuncs(
		func(msg string) detector.DetectionResult {
			detectCalled = true
			return detector.DetectionResult{IsCommitment: false}
		},
		func(sessionKey, message string) ([]string, error) { return nil, nil },
		func(filter beads.Filter) ([]beads.Bead, error) { return nil, nil },
		func(beadID string) (beads.Bead, error) { return beads.Bead{}, nil },
		func(beadID, reason string) error { return nil },
		func(commitmentID string, detectedAt time.Time, callback func(grace.VerificationOutcome)) {},
	)

	reqBody := []byte(`{"session_key":"s","message":"hello","role":"assistant"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/analyze", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()
	v2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !detectCalled {
		t.Fatal("expected detectCommitment to be called")
	}
}
