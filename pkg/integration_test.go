package pkg_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/perttulands/oathkeeper/pkg/api"
	"github.com/perttulands/oathkeeper/pkg/beads"
	"github.com/perttulands/oathkeeper/pkg/detector"
	"github.com/perttulands/oathkeeper/pkg/grace"
)

func requireBR(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("br"); err != nil {
		t.Skip("br not in PATH, skipping integration test")
	}
}

func brListWorks(t *testing.T) bool {
	t.Helper()
	store := beads.NewBeadStore("br")
	_, err := store.List(beads.Filter{Status: "open"})
	return err == nil
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

func startTestServer(t *testing.T, addr string, beadStore *beads.BeadStore) *http.Server {
	t.Helper()

	det := detector.NewDetector()
	gp := grace.New(100*time.Millisecond, func(detectedAt time.Time) (*grace.VerificationOutcome, error) {
		return &grace.VerificationOutcome{IsBacked: false}, nil
	})

	v2 := api.NewV2API(det, beadStore, gp)
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

func TestIntegrationFullLifecycle(t *testing.T) {
	requireBR(t)

	addr := freePort(t)
	var store *beads.BeadStore
	beadStoreAvailable := brListWorks(t)
	if beadStoreAvailable {
		store = beads.NewBeadStore("br")
	}
	server := startTestServer(t, addr, store)
	defer server.Close()

	baseURL := fmt.Sprintf("http://%s", addr)

	// Step 1: POST /api/v2/analyze with a commitment message
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
	if ar.Confidence < 0.90 {
		t.Fatalf("expected confidence >= 0.90, got %v", ar.Confidence)
	}

	// Step 2: Wait briefly for grace period (100ms in test)
	time.Sleep(300 * time.Millisecond)

	// Step 3: Verify commitments list endpoint responds
	if beadStoreAvailable {
		resp2, err := http.Get(baseURL + "/api/v2/commitments?status=open")
		if err != nil {
			t.Fatalf("list commitments failed: %v", err)
		}
		resp2.Body.Close()
		if resp2.StatusCode != http.StatusOK {
			t.Logf("commitments list returned %d (br may not be initialized)", resp2.StatusCode)
		}
	}

	// Step 4: POST analyze with a non-commitment message
	body2, _ := json.Marshal(analyzeReq{
		SessionKey: "integration-test",
		Message:    "Here's the status update on the deployment",
		Role:       "assistant",
	})

	resp3, err := http.Post(baseURL+"/api/v2/analyze", "application/json", bytes.NewReader(body2))
	if err != nil {
		t.Fatalf("non-commitment analyze request failed: %v", err)
	}
	defer resp3.Body.Close()

	var ar2 analyzeResp
	if err := json.NewDecoder(resp3.Body).Decode(&ar2); err != nil {
		t.Fatalf("decode non-commitment response: %v", err)
	}

	if ar2.Commitment {
		t.Fatal("expected commitment=false for status update message")
	}

	// Step 5: POST analyze with role=user → should be skipped
	body3, _ := json.Marshal(analyzeReq{
		SessionKey: "integration-test",
		Message:    "I'll check back in 5 minutes",
		Role:       "user",
	})

	resp4, err := http.Post(baseURL+"/api/v2/analyze", "application/json", bytes.NewReader(body3))
	if err != nil {
		t.Fatalf("user role analyze request failed: %v", err)
	}
	defer resp4.Body.Close()

	var ar3 analyzeResp
	if err := json.NewDecoder(resp4.Body).Decode(&ar3); err != nil {
		t.Fatalf("decode user role response: %v", err)
	}

	if ar3.Commitment {
		t.Fatal("expected commitment=false for user role")
	}

	// Step 6: Verify stats endpoint responds
	resp5, err := http.Get(baseURL + "/api/v2/stats")
	if err != nil {
		t.Fatalf("stats request failed: %v", err)
	}
	resp5.Body.Close()
	// Stats may return 500 if BeadStore is nil, but endpoint should respond
	if resp5.StatusCode != http.StatusOK && resp5.StatusCode != http.StatusInternalServerError {
		t.Fatalf("unexpected status from stats: %d", resp5.StatusCode)
	}
}

func TestIntegrationConcurrentCommitments(t *testing.T) {
	requireBR(t)

	addr := freePort(t)
	// Use nil beadStore for concurrency test — only testing detection throughput
	server := startTestServer(t, addr, nil)
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
}
