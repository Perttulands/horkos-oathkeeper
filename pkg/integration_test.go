package pkg_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/perttulands/oathkeeper/pkg/api"
	"github.com/perttulands/oathkeeper/pkg/beads"
	"github.com/perttulands/oathkeeper/pkg/detector"
	"github.com/perttulands/oathkeeper/pkg/grace"
)

func requireBD(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not in PATH, skipping integration test")
	}
}

func newIntegrationBeadStore(t *testing.T) *beads.BeadStore {
	t.Helper()

	brPath, err := exec.LookPath("bd")
	if err != nil {
		t.Skip("bd not in PATH")
	}

	workspace := t.TempDir()
	beadsDir := filepath.Join(workspace, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("create beads dir: %v", err)
	}

	dbPath := filepath.Join(beadsDir, "beads.db")
	wrapperPath := filepath.Join(workspace, "bd-wrapper.sh")
	wrapper := "#!/bin/sh\nBD=\"" + brPath + "\"\nDB=\"" + dbPath + "\"\nexec \"$BD\" --db \"$DB\" \"$@\"\n"
	if err := os.WriteFile(wrapperPath, []byte(wrapper), 0o755); err != nil {
		t.Fatalf("write wrapper script: %v", err)
	}

	return beads.NewBeadStore(wrapperPath)
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

func startIntegrationServer(t *testing.T, addr string, store *beads.BeadStore, graceDuration time.Duration) *http.Server {
	t.Helper()

	det := detector.NewDetector()
	gp := grace.New(graceDuration, func(detectedAt time.Time) (*grace.VerificationOutcome, error) {
		return &grace.VerificationOutcome{IsBacked: false}, nil
	})

	v2 := api.NewV2API(det, store, gp)

	// Wire grace callback to create beads for unbacked commitments
	v2.SetGraceCallback(func(commitmentID string, message string, category string, outcome grace.VerificationOutcome) {
		if !outcome.IsBacked && store != nil {
			_, _ = store.Create(beads.CommitmentInfo{
				Text:       message,
				Category:   category,
				SessionKey: commitmentID, // session key is embedded in commitmentID
				DetectedAt: time.Now().UTC(),
				ExpiresAt:  time.Now().UTC().Add(5 * time.Minute),
			})
		}
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

	// Wait for ready
	for i := 0; i < 100; i++ {
		time.Sleep(10 * time.Millisecond)
		resp, err := http.Get(fmt.Sprintf("http://%s/healthz", addr))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return server
			}
		}
	}
	t.Fatal("server did not become ready")
	return nil
}

type analyzeReq struct {
	SessionKey string `json:"session_key"`
	Message    string `json:"message"`
	Role       string `json:"role"`
}

type analyzeResp struct {
	Commitment bool     `json:"commitment"`
	Category   string   `json:"category"`
	Confidence float64  `json:"confidence"`
	Text       string   `json:"text"`
	Resolved   []string `json:"resolved"`
}

type statsResp struct {
	Total      int            `json:"total"`
	Open       int            `json:"open"`
	Resolved   int            `json:"resolved"`
	ByCategory map[string]int `json:"by_category"`
}

type commitmentResp struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	CloseReason string `json:"close_reason"`
}

func TestIntegrationFullLifecycle(t *testing.T) {
	requireBD(t)
	store := newIntegrationBeadStore(t)

	addr := freePort(t)
	server := startIntegrationServer(t, addr, store, 100*time.Millisecond)
	defer server.Close()

	baseURL := fmt.Sprintf("http://%s", addr)

	// Step 1: POST /api/v2/analyze with a temporal commitment
	body, _ := json.Marshal(analyzeReq{
		SessionKey: "integration-test",
		Message:    "I'll check back in 5 minutes to verify the deployment",
		Role:       "assistant",
	})

	resp, err := http.Post(baseURL+"/api/v2/analyze", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("analyze request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var ar analyzeResp
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		t.Fatalf("decode analyze response: %v", err)
	}

	if !ar.Commitment {
		t.Fatal("expected commitment=true")
	}
	if ar.Category != "temporal" {
		t.Fatalf("expected category temporal, got %q", ar.Category)
	}

	// Step 2: Wait for grace period (100ms) + margin
	time.Sleep(400 * time.Millisecond)

	// Step 3: GET /api/v2/commitments?status=open — verify bead was created
	resp2, err := http.Get(baseURL + "/api/v2/commitments?status=open")
	if err != nil {
		t.Fatalf("list commitments failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("list commitments expected 200, got %d", resp2.StatusCode)
	}

	var commitments []commitmentResp
	if err := json.NewDecoder(resp2.Body).Decode(&commitments); err != nil {
		t.Fatalf("decode commitments: %v", err)
	}

	if len(commitments) == 0 {
		t.Fatal("expected at least 1 open commitment after grace period")
	}

	beadID := commitments[0].ID
	if beadID == "" {
		t.Fatal("bead ID is empty")
	}

	// Step 4: POST /api/v2/commitments/:id/resolve
	resolveBody, _ := json.Marshal(map[string]string{"reason": "deployment verified successfully"})
	resp3, err := http.Post(
		fmt.Sprintf("%s/api/v2/commitments/%s/resolve", baseURL, beadID),
		"application/json",
		bytes.NewReader(resolveBody),
	)
	if err != nil {
		t.Fatalf("resolve request failed: %v", err)
	}
	defer resp3.Body.Close()

	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("resolve expected 200, got %d", resp3.StatusCode)
	}

	// Step 5: GET /api/v2/commitments/:id — verify closed
	resp4, err := http.Get(fmt.Sprintf("%s/api/v2/commitments/%s", baseURL, beadID))
	if err != nil {
		t.Fatalf("get commitment failed: %v", err)
	}
	defer resp4.Body.Close()

	if resp4.StatusCode != http.StatusOK {
		t.Fatalf("get commitment expected 200, got %d", resp4.StatusCode)
	}

	var detail commitmentResp
	if err := json.NewDecoder(resp4.Body).Decode(&detail); err != nil {
		t.Fatalf("decode commitment detail: %v", err)
	}

	if detail.Status != "closed" {
		t.Fatalf("expected status closed, got %q", detail.Status)
	}
	if detail.CloseReason == "" {
		t.Fatal("expected non-empty close reason after resolve")
	}

	// Step 6: GET /api/v2/stats — verify resolved count
	resp5, err := http.Get(baseURL + "/api/v2/stats")
	if err != nil {
		t.Fatalf("stats request failed: %v", err)
	}
	defer resp5.Body.Close()

	if resp5.StatusCode != http.StatusOK {
		t.Fatalf("stats expected 200, got %d", resp5.StatusCode)
	}

	var stats statsResp
	if err := json.NewDecoder(resp5.Body).Decode(&stats); err != nil {
		t.Fatalf("decode stats: %v", err)
	}

	if stats.Total < 1 {
		t.Fatalf("expected total >= 1, got %d", stats.Total)
	}
	if stats.Resolved < 1 {
		t.Fatalf("expected resolved >= 1, got %d", stats.Resolved)
	}
}

func TestIntegrationConcurrentCommitments(t *testing.T) {
	requireBD(t)
	store := newIntegrationBeadStore(t)

	addr := freePort(t)
	server := startIntegrationServer(t, addr, store, 100*time.Millisecond)
	defer server.Close()

	baseURL := fmt.Sprintf("http://%s", addr)

	// 5 goroutines posting commitments simultaneously
	var wg sync.WaitGroup
	errs := make(chan error, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			body, _ := json.Marshal(analyzeReq{
				SessionKey: fmt.Sprintf("concurrent-test-%d", idx),
				Message:    fmt.Sprintf("I'll check back in %d minutes", idx+1),
				Role:       "assistant",
			})

			resp, err := http.Post(baseURL+"/api/v2/analyze", "application/json", bytes.NewReader(body))
			if err != nil {
				errs <- fmt.Errorf("goroutine %d: request failed: %w", idx, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				errs <- fmt.Errorf("goroutine %d: expected 200, got %d", idx, resp.StatusCode)
				return
			}

			var ar analyzeResp
			if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
				errs <- fmt.Errorf("goroutine %d: decode failed: %w", idx, err)
				return
			}

			if !ar.Commitment {
				errs <- fmt.Errorf("goroutine %d: expected commitment=true", idx)
				return
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}

	// Wait for grace periods to fire
	time.Sleep(500 * time.Millisecond)

	// Verify distinct beads created
	openBeads, err := store.List(beads.Filter{Status: "open"})
	if err != nil {
		t.Fatalf("list open beads: %v", err)
	}

	if len(openBeads) < 5 {
		t.Fatalf("expected at least 5 distinct open beads, got %d", len(openBeads))
	}

	// Verify all IDs are distinct
	seen := make(map[string]bool)
	for _, b := range openBeads {
		if seen[b.ID] {
			t.Fatalf("duplicate bead ID: %s", b.ID)
		}
		seen[b.ID] = true
	}
}
