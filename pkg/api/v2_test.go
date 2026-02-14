package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
