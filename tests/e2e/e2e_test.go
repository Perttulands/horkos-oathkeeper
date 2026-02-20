// Package e2e provides end-to-end tests for Oathkeeper v2.
//
// Tests spin up a real HTTP server on a random port (no httptest.Server) and
// exercise the full request lifecycle: commitment detection, grace period,
// bead creation, context-aware auto-resolution, stats, and error cases.
//
// No external dependencies (br CLI) are required — an in-memory bead store
// backs all operations.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/perttulands/oathkeeper/pkg/api"
	"github.com/perttulands/oathkeeper/pkg/beads"
	"github.com/perttulands/oathkeeper/pkg/detector"
	"github.com/perttulands/oathkeeper/pkg/grace"
)

var e2eHTTPClient = &http.Client{Timeout: 5 * time.Second}

// ---------------------------------------------------------------------------
// In-memory bead store — replaces br CLI for tests
// ---------------------------------------------------------------------------

type memBead struct {
	id          string
	title       string
	status      string
	tags        []string
	createdAt   time.Time
	closedAt    time.Time
	closeReason string
}

type memBeadStore struct {
	mu    sync.Mutex
	beads map[string]*memBead
	seq   int
}

func newMemBeadStore() *memBeadStore {
	return &memBeadStore{beads: make(map[string]*memBead)}
}

func (m *memBeadStore) create(info beads.CommitmentInfo) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := fmt.Sprintf("bd-%04d", m.seq)
	tags := []string{"oathkeeper"}
	if info.Category != "" {
		tags = append(tags, info.Category)
	}
	if info.SessionKey != "" {
		tags = append(tags, "session-"+sanitize(info.SessionKey))
	}
	m.beads[id] = &memBead{
		id:        id,
		title:     "oathkeeper: " + info.Text,
		status:    "open",
		tags:      tags,
		createdAt: info.DetectedAt,
	}
	return id, nil
}

func (m *memBeadStore) list(filter beads.Filter) ([]beads.Bead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []beads.Bead
	for _, b := range m.beads {
		if filter.Status != "" && b.status != filter.Status {
			continue
		}
		if filter.Category != "" && !contains(b.tags, filter.Category) {
			continue
		}
		out = append(out, toBead(b))
	}
	// Stable ordering for tests
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *memBeadStore) get(id string) (beads.Bead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.beads[id]
	if !ok {
		return beads.Bead{}, beads.ErrBeadNotFound
	}
	return toBead(b), nil
}

func (m *memBeadStore) resolve(id, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.beads[id]
	if !ok {
		return beads.ErrBeadNotFound
	}
	b.status = "closed"
	b.closedAt = time.Now().UTC()
	b.closeReason = reason
	return nil
}

func (m *memBeadStore) autoResolve(sessionKey, message string) ([]string, error) {
	lower := strings.ToLower(message)
	hasIndicator := false
	for _, kw := range []string{"i checked", "done", "completed", "here are the results"} {
		if strings.Contains(lower, kw) {
			hasIndicator = true
			break
		}
	}
	if !hasIndicator {
		return []string{}, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	sessionTag := "session-" + sanitize(sessionKey)
	var resolved []string
	for _, b := range m.beads {
		if b.status != "open" {
			continue
		}
		if !contains(b.tags, sessionTag) {
			continue
		}
		b.status = "closed"
		b.closedAt = time.Now().UTC()
		b.closeReason = message
		resolved = append(resolved, b.id)
	}
	return resolved, nil
}

func toBead(m *memBead) beads.Bead {
	return beads.Bead{
		ID:          m.id,
		Title:       m.title,
		Status:      m.status,
		Tags:        m.tags,
		CreatedAt:   m.createdAt,
		ClosedAt:    m.closedAt,
		CloseReason: m.closeReason,
	}
}

func contains(tags []string, want string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, want) {
			return true
		}
	}
	return false
}

func sanitize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, ch := range s {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			b.WriteRune(ch)
		} else {
			b.WriteRune('-')
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Test server helpers
// ---------------------------------------------------------------------------

// testEnv bundles everything needed for an E2E test.
type testEnv struct {
	baseURL string
	server  *http.Server
	store   *memBeadStore
	grace   *grace.GracePeriod
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

// startEnv spins up a real HTTP server with the given grace period duration.
func startEnv(t *testing.T, graceDuration time.Duration) *testEnv {
	t.Helper()

	store := newMemBeadStore()
	det := detector.NewDetector()
	gp := grace.New(graceDuration, func(detectedAt time.Time) (*grace.VerificationOutcome, error) {
		// Always return "not backed" so beads get created.
		return &grace.VerificationOutcome{IsBacked: false}, nil
	})

	v2 := api.NewV2API(det, nil, gp)

	// Override bead-related functions to use the in-memory store.
	v2.SetResolveBead(store.resolve)

	// We need to wire listBeads, getBead, autoResolve — but V2API doesn't expose
	// setters for all of them. Instead, we'll build a V2API from scratch with
	// our own function closures. We can use the approach the existing unit tests
	// use: construct a V2API struct directly (exported fields are functions).
	// Since the V2API fields are unexported, we use NewV2API + overrides or
	// construct via the api package's constructor and inject via exported setters.
	//
	// Looking at the V2API struct more carefully, we see that NewV2API accepts
	// *detector.Detector, *beads.BeadStore, *grace.GracePeriod. The internal
	// function fields are unexported. The unit tests set them directly because
	// they're in the same package. From outside the package, we must use the
	// constructor approach.
	//
	// The simplest solution: create a V2API via NewV2API(det, nil, gp) and then
	// use SetResolveBead. For listBeads/getBead/autoResolve, since there's no
	// exported setter, we'll create the V2API using a wrapper approach.
	//
	// Actually, let me re-read the NewV2API signature — it takes *beads.BeadStore.
	// If we pass nil, listBeads/getBead/autoResolve are nil. We need them.
	// We can't create a real BeadStore without br CLI.
	//
	// Solution: Build the server using a custom handler that delegates to our
	// own implementation. But that defeats the purpose of E2E testing.
	//
	// Better solution: Use the internal test API builder pattern. Since the
	// V2API struct fields are unexported, but the tests in pkg/api use direct
	// struct initialization, we need to be in the api package to do that.
	//
	// Real E2E solution: Use the exported API surface. Let me check if there
	// are setters we missed...

	// After re-reading the code: the only exported setter is SetResolveBead,
	// SetGraceCallback, SetResolveCallback, SetContextAnalyzer.
	// For listBeads, getBead, autoResolve — no setters.
	//
	// We'll use a different approach: create a test helper in an _test.go file
	// within the api package that exports a constructor, or we'll use the
	// "httptest server wrapping a mux we build ourselves" approach.

	// Actually, the cleanest approach is to build our own mux that mirrors
	// what serve.go does, using the V2API handler, but have a wrapper around
	// it. But wait — V2API.Handler() returns a handler, and NewV2API with nil
	// BeadStore means listBeads/getBead/autoResolve are nil (those endpoints
	// return 500 "bead store unavailable").
	//
	// Let me look at this differently: we CAN build a complete test by using
	// the internal test helper buildV2API that creates a testable V2API.
	// Since we're in a separate package, we need to use exported API only.
	//
	// The pragmatic approach: build a thin HTTP handler that wraps V2API for
	// detection + grace, and adds our own bead endpoints backed by memBeadStore.
	// This tests the real detector, real grace period, real HTTP transport, but
	// uses a fake bead store.

	// FINAL APPROACH: We'll build a custom mux in this test file that uses:
	// - V2API.Handler() for /api/v2/analyze (detection + grace + context)
	// - Our own handlers for commitments/stats/resolve backed by memBeadStore
	//
	// Wait — but that means we're not testing the REAL commitments/stats code.
	//
	// SIMPLEST CORRECT APPROACH: Use NewV2APIForTest exported from api package.
	// But that doesn't exist.
	//
	// OK let me just do this properly. We'll create a small exported helper
	// in the api package.

	// Actually the simplest thing: just create a wrapped server here that uses
	// the real V2API handler, but since beadStore is nil, the bead-related
	// endpoints (commitments, stats, resolve) won't work via the V2API handler.
	// We'll add our own handlers for those endpoints using memBeadStore.
	//
	// For the /api/v2/analyze endpoint, we need autoResolve to work, but it's
	// nil when beadStore is nil. That means auto-resolve via analyze won't work.
	//
	// Let me take yet another approach: create a TestableV2API helper file in
	// the api package that's only compiled during tests.

	// DECISION: I'll create a small exported builder in tests/e2e/helpers_test.go
	// that constructs the test server using the api package's exported types.
	// Since the api package doesn't expose the function fields, I'll create
	// a file in pkg/api/ called testhelper_test.go... no, that would only be
	// accessible from within pkg/api tests.
	//
	// FINAL DECISION: I'll create a file pkg/api/export_test.go that exports
	// test helpers via an _test.go file in the api package with an exported
	// function. This is Go's standard pattern for "white-box testing from
	// external packages".

	_ = v2
	_ = store
	_ = gp
	_ = det

	t.Fatal("this code path is replaced by buildTestServer")
	return nil
}

// buildTestServer creates a server with all wiring done via the exported API.
// It uses the api.NewTestV2API helper exported from pkg/api/export_test.go.
func buildTestServer(t *testing.T, graceDuration time.Duration) *testEnv {
	t.Helper()

	store := newMemBeadStore()
	addr := freePort(t)

	det := detector.NewDetector()
	gp := grace.New(graceDuration, func(detectedAt time.Time) (*grace.VerificationOutcome, error) {
		return &grace.VerificationOutcome{IsBacked: false}, nil
	})

	v2 := api.NewV2APIWithFuncs(
		det.DetectCommitment,
		store.autoResolve,
		store.list,
		store.get,
		store.resolve,
		gp.Schedule,
	)

	// Wire context analyzer
	ca := detector.NewContextAnalyzer(5)
	v2.SetContextAnalyzer(ca, 5)

	// Wire grace callback: create bead when commitment is unbacked
	v2.SetGraceCallback(func(meta api.GraceCallbackContext, outcome grace.VerificationOutcome) {
		if outcome.IsBacked {
			return
		}
		store.create(beads.CommitmentInfo{
			Text:       meta.Message,
			Category:   meta.Category,
			SessionKey: meta.SessionKey,
			DetectedAt: meta.DetectedAt,
		})
	})

	mux := http.NewServeMux()
	v2Handler := v2.Handler()
	mux.Handle("/api/v2/", v2Handler)
	mux.Handle("/api/v2/analyze", v2Handler)
	mux.Handle("/api/v2/stats", v2Handler)
	mux.Handle("/api/v2/commitments", v2Handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok"}`)
	})

	server := &http.Server{Addr: addr, Handler: mux}

	go server.ListenAndServe()

	// Wait for server ready
	client := &http.Client{Timeout: 2 * time.Second}
	for i := 0; i < 100; i++ {
		time.Sleep(10 * time.Millisecond)
		resp, err := client.Get(fmt.Sprintf("http://%s/healthz", addr))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				t.Cleanup(func() {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()
					server.Shutdown(ctx)
					gp.Stop()
				})
				return &testEnv{
					baseURL: fmt.Sprintf("http://%s", addr),
					server:  server,
					store:   store,
					grace:   gp,
				}
			}
		}
	}
	t.Fatal("server did not become ready within 1s")
	return nil
}

// ---------------------------------------------------------------------------
// JSON request/response types
// ---------------------------------------------------------------------------

type analyzeReq struct {
	SessionKey string `json:"session_key"`
	Message    string `json:"message"`
	Role       string `json:"role"`
}

type analyzeResp struct {
	Commitment bool    `json:"commitment"`
	Category   string  `json:"category"`
	Confidence float64 `json:"confidence"`
	Text       string  `json:"text"`
	Resolved   []string          `json:"resolved"`
	Fulfilled  []fulfilledResp   `json:"fulfilled"`
	Escalated  []escalatedResp   `json:"escalated"`
}

type fulfilledResp struct {
	Text        string `json:"text"`
	FulfilledBy string `json:"fulfilled_by"`
}

type escalatedResp struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

type commitmentResp struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	Tags        []string  `json:"tags"`
	CreatedAt   time.Time `json:"created_at"`
	ClosedAt    time.Time `json:"closed_at,omitempty"`
	CloseReason string    `json:"close_reason,omitempty"`
}

type statsResp struct {
	Total      int            `json:"total"`
	Open       int            `json:"open"`
	Resolved   int            `json:"resolved"`
	ByCategory map[string]int `json:"by_category"`
}

type errorResp struct {
	Error string `json:"error"`
}

type resolveResp struct {
	ID       string `json:"id"`
	Resolved bool   `json:"resolved"`
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

func postJSON(t *testing.T, url string, body interface{}) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	resp, err := e2eHTTPClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func getJSON(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := e2eHTTPClient.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

func decodeJSON[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return v
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestE2E_CommitmentDetection verifies that POSTing an assistant message with
// commitment language returns commitment=true with the correct category.
func TestE2E_CommitmentDetection(t *testing.T) {
	env := buildTestServer(t, 50*time.Millisecond)

	cases := []struct {
		name     string
		message  string
		wantCommitment bool
		wantCategory   string
	}{
		{
			name:           "temporal commitment",
			message:        "I'll check back in 5 minutes to verify the deployment",
			wantCommitment: true,
			wantCategory:   "temporal",
		},
		{
			name:           "followup commitment",
			message:        "I'll monitor the build output for errors",
			wantCommitment: true,
			wantCategory:   "followup",
		},
		{
			name:           "conditional commitment",
			message:        "Once the tests pass, I'll deploy to production",
			wantCommitment: true,
			wantCategory:   "conditional",
		},
		{
			name:           "no commitment — status update",
			message:        "The deployment is running smoothly right now",
			wantCommitment: false,
		},
		{
			name:           "no commitment — past tense",
			message:        "I already checked the logs",
			wantCommitment: false,
		},
		{
			name:           "non-assistant role ignored",
			message:        "I'll check back in 5 minutes",
			wantCommitment: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			role := "assistant"
			if tc.name == "non-assistant role ignored" {
				role = "user"
			}

			resp := postJSON(t, env.baseURL+"/api/v2/analyze", analyzeReq{
				SessionKey: "detect-" + tc.name,
				Message:    tc.message,
				Role:       role,
			})

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", resp.StatusCode, readBody(t, resp))
			}

			ar := decodeJSON[analyzeResp](t, resp)

			if ar.Commitment != tc.wantCommitment {
				t.Fatalf("commitment: got %v, want %v", ar.Commitment, tc.wantCommitment)
			}
			if tc.wantCommitment && ar.Category != tc.wantCategory {
				t.Fatalf("category: got %q, want %q", ar.Category, tc.wantCategory)
			}
		})
	}
}

// TestE2E_FullLifecycle tests: detect → grace period → bead creation → list → resolve → verify closed → stats.
func TestE2E_FullLifecycle(t *testing.T) {
	env := buildTestServer(t, 50*time.Millisecond)

	// Step 1: POST a temporal commitment
	resp := postJSON(t, env.baseURL+"/api/v2/analyze", analyzeReq{
		SessionKey: "lifecycle-test",
		Message:    "I'll check back in 10 minutes to verify everything",
		Role:       "assistant",
	})
	ar := decodeJSON[analyzeResp](t, resp)
	if !ar.Commitment {
		t.Fatal("expected commitment=true")
	}
	if ar.Category != "temporal" {
		t.Fatalf("expected category temporal, got %q", ar.Category)
	}

	// Step 2: Wait for grace period to fire + margin
	time.Sleep(200 * time.Millisecond)

	// Step 3: List open commitments — should have at least 1
	resp2 := getJSON(t, env.baseURL+"/api/v2/commitments?status=open")
	commitments := decodeJSON[[]commitmentResp](t, resp2)
	if len(commitments) == 0 {
		t.Fatal("expected at least 1 open commitment after grace period")
	}

	beadID := commitments[0].ID
	if beadID == "" {
		t.Fatal("bead ID is empty")
	}
	if commitments[0].Status != "open" {
		t.Fatalf("expected status open, got %q", commitments[0].Status)
	}

	// Step 4: GET the specific commitment
	resp3 := getJSON(t, fmt.Sprintf("%s/api/v2/commitments/%s", env.baseURL, beadID))
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("get commitment: expected 200, got %d", resp3.StatusCode)
	}
	detail := decodeJSON[commitmentResp](t, resp3)
	if detail.ID != beadID {
		t.Fatalf("expected id %s, got %s", beadID, detail.ID)
	}

	// Step 5: Resolve the commitment
	resp4 := postJSON(t, fmt.Sprintf("%s/api/v2/commitments/%s/resolve", env.baseURL, beadID),
		map[string]string{"reason": "deployment verified successfully"})
	if resp4.StatusCode != http.StatusOK {
		body := readBody(t, resp4)
		t.Fatalf("resolve: expected 200, got %d: %s", resp4.StatusCode, body)
	}
	rr := decodeJSON[resolveResp](t, resp4)
	if !rr.Resolved {
		t.Fatal("expected resolved=true")
	}

	// Step 6: Verify the commitment is now closed
	resp5 := getJSON(t, fmt.Sprintf("%s/api/v2/commitments/%s", env.baseURL, beadID))
	closed := decodeJSON[commitmentResp](t, resp5)
	if closed.Status != "closed" {
		t.Fatalf("expected status closed, got %q", closed.Status)
	}
	if closed.CloseReason == "" {
		t.Fatal("expected non-empty close reason")
	}

	// Step 7: Verify stats
	resp6 := getJSON(t, env.baseURL+"/api/v2/stats")
	stats := decodeJSON[statsResp](t, resp6)
	if stats.Total < 1 {
		t.Fatalf("expected total >= 1, got %d", stats.Total)
	}
	if stats.Resolved < 1 {
		t.Fatalf("expected resolved >= 1, got %d", stats.Resolved)
	}
}

// TestE2E_ContextAnalyzer_FulfillmentDetection sends a commitment, then a
// fulfillment message, and verifies the context analyzer detects it.
func TestE2E_ContextAnalyzer_FulfillmentDetection(t *testing.T) {
	env := buildTestServer(t, 5*time.Second) // long grace so bead isn't created yet

	session := "ctx-fulfill"

	// Message 1: commitment
	resp1 := postJSON(t, env.baseURL+"/api/v2/analyze", analyzeReq{
		SessionKey: session,
		Message:    "I need to check the logs for errors",
		Role:       "assistant",
	})
	ar1 := decodeJSON[analyzeResp](t, resp1)
	if !ar1.Commitment {
		// "I need to" is a weak commitment — should be detected
		t.Fatal("expected commitment=true for 'I need to check'")
	}

	// Message 2: unrelated filler
	postJSON(t, env.baseURL+"/api/v2/analyze", analyzeReq{
		SessionKey: session,
		Message:    "Looking at the deployment status now",
		Role:       "assistant",
	})

	// Message 3: fulfillment
	resp3 := postJSON(t, env.baseURL+"/api/v2/analyze", analyzeReq{
		SessionKey: session,
		Message:    "I checked the logs and found the issue",
		Role:       "assistant",
	})
	ar3 := decodeJSON[analyzeResp](t, resp3)

	if len(ar3.Fulfilled) == 0 {
		t.Fatal("expected fulfilled commitments in response, got none")
	}

	found := false
	for _, f := range ar3.Fulfilled {
		if strings.Contains(f.FulfilledBy, "checked the logs") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected fulfillment mentioning 'checked the logs', got %+v", ar3.Fulfilled)
	}
}

// TestE2E_ContextAnalyzer_Escalation sends repeated commitments of the same
// type and verifies escalation is reported.
func TestE2E_ContextAnalyzer_Escalation(t *testing.T) {
	env := buildTestServer(t, 5*time.Second)

	session := "ctx-escalate"

	// Two followup-type commitments in same session
	postJSON(t, env.baseURL+"/api/v2/analyze", analyzeReq{
		SessionKey: session,
		Message:    "I'll monitor the build output",
		Role:       "assistant",
	})

	resp := postJSON(t, env.baseURL+"/api/v2/analyze", analyzeReq{
		SessionKey: session,
		Message:    "I'll watch the deployment logs",
		Role:       "assistant",
	})
	ar := decodeJSON[analyzeResp](t, resp)

	if len(ar.Escalated) == 0 {
		t.Fatal("expected escalated commitments, got none")
	}
	if ar.Escalated[0].Count < 2 {
		t.Fatalf("expected escalation count >= 2, got %d", ar.Escalated[0].Count)
	}
}

// TestE2E_ContextAnalyzer_SessionIsolation verifies that context buffers
// are per-session and don't leak between sessions.
func TestE2E_ContextAnalyzer_SessionIsolation(t *testing.T) {
	env := buildTestServer(t, 5*time.Second)

	// Session A: commitment
	postJSON(t, env.baseURL+"/api/v2/analyze", analyzeReq{
		SessionKey: "session-a",
		Message:    "I'll check the logs for errors",
		Role:       "assistant",
	})

	// Session B: fulfillment text — should NOT match session A's commitment
	resp := postJSON(t, env.baseURL+"/api/v2/analyze", analyzeReq{
		SessionKey: "session-b",
		Message:    "I checked the logs and found the issue",
		Role:       "assistant",
	})
	ar := decodeJSON[analyzeResp](t, resp)

	if len(ar.Fulfilled) != 0 {
		t.Fatalf("expected no fulfilled (different session), got %+v", ar.Fulfilled)
	}
}

// TestE2E_StatsAccuracy creates multiple commitments, resolves some, and
// verifies the stats endpoint returns accurate counts.
func TestE2E_StatsAccuracy(t *testing.T) {
	env := buildTestServer(t, 50*time.Millisecond)

	messages := []struct {
		session string
		message string
	}{
		{"stats-1", "I'll check back in 5 minutes"},
		{"stats-2", "I'll check back in 10 minutes"},
		{"stats-3", "I'll monitor the build output"},
	}

	for _, m := range messages {
		resp := postJSON(t, env.baseURL+"/api/v2/analyze", analyzeReq{
			SessionKey: m.session,
			Message:    m.message,
			Role:       "assistant",
		})
		ar := decodeJSON[analyzeResp](t, resp)
		if !ar.Commitment {
			t.Fatalf("expected commitment for %q", m.message)
		}
	}

	// Wait for grace periods
	time.Sleep(300 * time.Millisecond)

	// Verify 3 open beads
	resp := getJSON(t, env.baseURL+"/api/v2/commitments?status=open")
	commitments := decodeJSON[[]commitmentResp](t, resp)
	if len(commitments) < 3 {
		t.Fatalf("expected >= 3 open commitments, got %d", len(commitments))
	}

	// Resolve the first one
	postJSON(t, fmt.Sprintf("%s/api/v2/commitments/%s/resolve", env.baseURL, commitments[0].ID),
		map[string]string{"reason": "done"})

	// Check stats
	resp2 := getJSON(t, env.baseURL+"/api/v2/stats")
	stats := decodeJSON[statsResp](t, resp2)

	if stats.Total < 3 {
		t.Fatalf("expected total >= 3, got %d", stats.Total)
	}
	if stats.Open < 2 {
		t.Fatalf("expected open >= 2, got %d", stats.Open)
	}
	if stats.Resolved < 1 {
		t.Fatalf("expected resolved >= 1, got %d", stats.Resolved)
	}
	if len(stats.ByCategory) == 0 {
		t.Fatal("expected non-empty by_category")
	}
}

// TestE2E_ConcurrentRequests fires multiple commitment requests simultaneously
// and verifies all are processed without races or lost data.
func TestE2E_ConcurrentRequests(t *testing.T) {
	env := buildTestServer(t, 50*time.Millisecond)

	const numRequests = 10
	var wg sync.WaitGroup
	errs := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			resp := postJSON(t, env.baseURL+"/api/v2/analyze", analyzeReq{
				SessionKey: fmt.Sprintf("concurrent-%d", idx),
				Message:    fmt.Sprintf("I'll check back in %d minutes", idx+1),
				Role:       "assistant",
			})
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				errs <- fmt.Errorf("request %d: expected 200, got %d", idx, resp.StatusCode)
				return
			}

			var ar analyzeResp
			if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
				errs <- fmt.Errorf("request %d: decode failed: %v", idx, err)
				return
			}

			if !ar.Commitment {
				errs <- fmt.Errorf("request %d: expected commitment=true", idx)
				return
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}

	// Wait for all grace periods to fire
	time.Sleep(300 * time.Millisecond)

	// Verify correct number of beads created
	env.store.mu.Lock()
	beadCount := len(env.store.beads)
	env.store.mu.Unlock()

	if beadCount < numRequests {
		t.Fatalf("expected >= %d beads, got %d", numRequests, beadCount)
	}
}

// TestE2E_ErrorCases tests various error conditions.
func TestE2E_ErrorCases(t *testing.T) {
	env := buildTestServer(t, 50*time.Millisecond)

	t.Run("malformed JSON", func(t *testing.T) {
		resp, err := e2eHTTPClient.Post(env.baseURL+"/api/v2/analyze", "application/json",
			bytes.NewReader([]byte("{")))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}

		var errBody errorResp
		json.NewDecoder(resp.Body).Decode(&errBody)
		if errBody.Error == "" {
			t.Fatal("expected error message in response")
		}
	})

	t.Run("empty body", func(t *testing.T) {
		resp, err := e2eHTTPClient.Post(env.baseURL+"/api/v2/analyze", "application/json",
			bytes.NewReader([]byte("")))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("wrong method on analyze", func(t *testing.T) {
		resp, err := e2eHTTPClient.Get(env.baseURL + "/api/v2/analyze")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", resp.StatusCode)
		}
	})

	t.Run("wrong method on stats", func(t *testing.T) {
		resp, err := e2eHTTPClient.Post(env.baseURL+"/api/v2/stats", "application/json",
			bytes.NewReader([]byte("{}")))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", resp.StatusCode)
		}
	})

	t.Run("wrong method on commitments list", func(t *testing.T) {
		resp, err := e2eHTTPClient.Post(env.baseURL+"/api/v2/commitments", "application/json",
			bytes.NewReader([]byte("{}")))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", resp.StatusCode)
		}
	})

	t.Run("resolve nonexistent commitment", func(t *testing.T) {
		resp := postJSON(t, env.baseURL+"/api/v2/commitments/nonexistent-id/resolve",
			map[string]string{"reason": "test"})
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("resolve with empty reason", func(t *testing.T) {
		resp := postJSON(t, env.baseURL+"/api/v2/commitments/any-id/resolve",
			map[string]string{"reason": ""})
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for empty reason, got %d", resp.StatusCode)
		}
	})

	t.Run("get nonexistent commitment", func(t *testing.T) {
		resp := getJSON(t, env.baseURL+"/api/v2/commitments/does-not-exist")
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("resolve with malformed JSON", func(t *testing.T) {
		resp, err := e2eHTTPClient.Post(env.baseURL+"/api/v2/commitments/any/resolve",
			"application/json", bytes.NewReader([]byte("not json")))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})
}

// TestE2E_GracePeriodCreatesBeadOnly verifies the timing: commitment detection
// is immediate but bead creation happens after the grace period.
func TestE2E_GracePeriodCreatesBeadOnly(t *testing.T) {
	env := buildTestServer(t, 200*time.Millisecond) // 200ms grace

	// POST a commitment
	resp := postJSON(t, env.baseURL+"/api/v2/analyze", analyzeReq{
		SessionKey: "grace-timing",
		Message:    "I'll check back in 5 minutes",
		Role:       "assistant",
	})
	ar := decodeJSON[analyzeResp](t, resp)
	if !ar.Commitment {
		t.Fatal("expected commitment=true")
	}

	// Immediately after: no beads yet (grace period hasn't fired)
	env.store.mu.Lock()
	countBefore := len(env.store.beads)
	env.store.mu.Unlock()
	if countBefore != 0 {
		t.Fatalf("expected 0 beads before grace period, got %d", countBefore)
	}

	// Wait for grace period + margin
	time.Sleep(400 * time.Millisecond)

	// Now bead should exist
	env.store.mu.Lock()
	countAfter := len(env.store.beads)
	env.store.mu.Unlock()
	if countAfter != 1 {
		t.Fatalf("expected 1 bead after grace period, got %d", countAfter)
	}
}

// TestE2E_AutoResolveViaAnalyze tests that posting a resolution indicator
// message auto-resolves open beads for the same session.
func TestE2E_AutoResolveViaAnalyze(t *testing.T) {
	env := buildTestServer(t, 50*time.Millisecond)
	session := "auto-resolve-session"

	// Create a commitment
	resp := postJSON(t, env.baseURL+"/api/v2/analyze", analyzeReq{
		SessionKey: session,
		Message:    "I'll check back in 5 minutes",
		Role:       "assistant",
	})
	ar := decodeJSON[analyzeResp](t, resp)
	if !ar.Commitment {
		t.Fatal("expected commitment=true")
	}

	// Wait for grace period to create the bead
	time.Sleep(200 * time.Millisecond)

	// Verify bead exists and is open
	env.store.mu.Lock()
	openCount := 0
	for _, b := range env.store.beads {
		if b.status == "open" {
			openCount++
		}
	}
	env.store.mu.Unlock()
	if openCount == 0 {
		t.Fatal("expected at least 1 open bead before auto-resolve")
	}

	// Send a message with resolution indicator
	resp2 := postJSON(t, env.baseURL+"/api/v2/analyze", analyzeReq{
		SessionKey: session,
		Message:    "I checked and here are the results of the deployment",
		Role:       "assistant",
	})
	ar2 := decodeJSON[analyzeResp](t, resp2)

	if len(ar2.Resolved) == 0 {
		t.Fatal("expected resolved bead IDs in response")
	}
}

// TestE2E_HealthEndpoint verifies /healthz returns 200 OK.
func TestE2E_HealthEndpoint(t *testing.T) {
	env := buildTestServer(t, 50*time.Millisecond)

	resp := getJSON(t, env.baseURL+"/healthz")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()

	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %q", body["status"])
	}
}

// TestE2E_EmptyStatsOnFreshServer verifies stats are zeroed on a fresh server.
func TestE2E_EmptyStatsOnFreshServer(t *testing.T) {
	env := buildTestServer(t, 50*time.Millisecond)

	resp := getJSON(t, env.baseURL+"/api/v2/stats")
	stats := decodeJSON[statsResp](t, resp)

	if stats.Total != 0 || stats.Open != 0 || stats.Resolved != 0 {
		t.Fatalf("expected all-zero stats on fresh server, got %+v", stats)
	}
}

// TestE2E_CommitmentsFilterByCategory tests the category filter on commitments list.
func TestE2E_CommitmentsFilterByCategory(t *testing.T) {
	env := buildTestServer(t, 50*time.Millisecond)

	// Create temporal commitment
	postJSON(t, env.baseURL+"/api/v2/analyze", analyzeReq{
		SessionKey: "cat-temporal",
		Message:    "I'll check back in 5 minutes",
		Role:       "assistant",
	})

	// Create followup commitment
	postJSON(t, env.baseURL+"/api/v2/analyze", analyzeReq{
		SessionKey: "cat-followup",
		Message:    "I'll monitor the deployment output",
		Role:       "assistant",
	})

	// Wait for grace periods
	time.Sleep(300 * time.Millisecond)

	// Filter by temporal
	resp := getJSON(t, env.baseURL+"/api/v2/commitments?category=temporal")
	commitments := decodeJSON[[]commitmentResp](t, resp)

	for _, c := range commitments {
		if !contains(c.Tags, "temporal") {
			t.Fatalf("expected all commitments to have temporal tag, got tags %v", c.Tags)
		}
	}
}

// TestE2E_MultipleCommitmentsInSequence verifies that multiple sequential
// commitments each create their own bead.
func TestE2E_MultipleCommitmentsInSequence(t *testing.T) {
	env := buildTestServer(t, 50*time.Millisecond)

	for i := 0; i < 5; i++ {
		postJSON(t, env.baseURL+"/api/v2/analyze", analyzeReq{
			SessionKey: fmt.Sprintf("seq-%d", i),
			Message:    fmt.Sprintf("I'll check back in %d minutes", i+1),
			Role:       "assistant",
		})
		// Small delay to ensure unique nanosecond timestamps in commitment IDs
		time.Sleep(5 * time.Millisecond)
	}

	// Wait for all grace periods
	time.Sleep(300 * time.Millisecond)

	env.store.mu.Lock()
	count := len(env.store.beads)
	env.store.mu.Unlock()

	if count != 5 {
		t.Fatalf("expected 5 beads, got %d", count)
	}
}
