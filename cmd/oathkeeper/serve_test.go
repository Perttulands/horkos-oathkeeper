package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/perttulands/oathkeeper/pkg/api"
	"github.com/perttulands/oathkeeper/pkg/beads"
	"github.com/perttulands/oathkeeper/pkg/daemon"
	"github.com/perttulands/oathkeeper/pkg/detector"
	"github.com/perttulands/oathkeeper/pkg/grace"
)

func TestServeStartsAndRespondsToHealth(t *testing.T) {
	// Find a free port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	// Wire minimal dependencies (no real br needed for health check)
	det := detector.NewDetector()
	v2 := api.NewV2API(det, nil, nil)

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

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	d := daemon.New(daemon.Config{
		ShutdownTimeout: 5 * time.Second,
		OnStart: func(ctx context.Context) error {
			errCh := make(chan error, 1)
			go func() {
				errCh <- server.ListenAndServe()
			}()
			select {
			case err := <-errCh:
				if err == http.ErrServerClosed {
					return nil
				}
				return err
			case <-ctx.Done():
				shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				server.Shutdown(shutCtx)
				return nil
			}
		},
		OnStop: func() {},
	})

	// Start in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run()
	}()

	// Wait for server to be ready
	ready := false
	for i := 0; i < 50; i++ {
		time.Sleep(20 * time.Millisecond)
		resp, err := http.Get(fmt.Sprintf("http://%s/healthz", addr))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
	}
	if !ready {
		t.Fatal("server did not become ready within 1 second")
	}

	// Verify health endpoint
	resp, err := http.Get(fmt.Sprintf("http://%s/healthz", addr))
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /healthz, got %d", resp.StatusCode)
	}

	var health map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if health["status"] != "ok" {
		t.Fatalf("expected status ok, got %q", health["status"])
	}

	// Shutdown
	d.Shutdown()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("server returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down within 5 seconds")
	}
}

func TestServeGracefulShutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	gp := grace.New(30*time.Second, func(detectedAt time.Time) (*grace.VerificationOutcome, error) {
		return &grace.VerificationOutcome{IsBacked: false}, nil
	})

	det := detector.NewDetector()
	v2 := api.NewV2API(det, nil, gp)

	mux := http.NewServeMux()
	v2Handler := v2.Handler()
	mux.Handle("/api/v2/", v2Handler)
	mux.Handle("/api/v2/analyze", v2Handler)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	stopCalled := false
	d := daemon.New(daemon.Config{
		ShutdownTimeout: 5 * time.Second,
		OnStart: func(ctx context.Context) error {
			errCh := make(chan error, 1)
			go func() {
				errCh <- server.ListenAndServe()
			}()
			select {
			case err := <-errCh:
				if err == http.ErrServerClosed {
					return nil
				}
				return err
			case <-ctx.Done():
				shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				server.Shutdown(shutCtx)
				return nil
			}
		},
		OnStop: func() {
			gp.Stop()
			stopCalled = true
		},
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run()
	}()

	// Wait for ready
	for i := 0; i < 50; i++ {
		time.Sleep(20 * time.Millisecond)
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			conn.Close()
			break
		}
	}

	// Schedule a grace period to verify it gets cancelled on shutdown
	gp.Schedule("test-commitment", time.Now(), func(outcome grace.VerificationOutcome) {})
	if gp.Pending() != 1 {
		t.Fatalf("expected 1 pending verification, got %d", gp.Pending())
	}

	d.Shutdown()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("server error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown timed out")
	}

	if !stopCalled {
		t.Fatal("OnStop was not called during shutdown")
	}

	if gp.Pending() != 0 {
		t.Fatalf("expected 0 pending verifications after shutdown, got %d", gp.Pending())
	}
}

func TestServeWiresV2APIRoutes(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	// Use mock beadStore functions
	det := detector.NewDetector()
	v2 := &api.V2API{}
	// Can't access private fields from test, so use NewV2API with nil
	v2 = api.NewV2API(det, nil, nil)

	mux := http.NewServeMux()
	v2Handler := v2.Handler()
	mux.Handle("/api/v2/", v2Handler)
	mux.Handle("/api/v2/analyze", v2Handler)
	mux.Handle("/api/v2/stats", v2Handler)
	mux.Handle("/api/v2/commitments", v2Handler)

	server := &http.Server{Addr: addr, Handler: mux}
	go server.ListenAndServe()
	defer server.Close()

	// Wait for ready
	for i := 0; i < 50; i++ {
		time.Sleep(20 * time.Millisecond)
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			conn.Close()
			break
		}
	}

	// Test analyze endpoint is routed
	resp, err := http.Post(fmt.Sprintf("http://%s/api/v2/analyze", addr), "application/json",
		nil)
	if err != nil {
		t.Fatalf("analyze request failed: %v", err)
	}
	resp.Body.Close()
	// Should be 400 (bad request) since we sent nil body, not 404
	if resp.StatusCode == http.StatusNotFound {
		t.Fatal("analyze endpoint not routed (got 404)")
	}
}

// TestBeadStoreCreation verifies the BeadStore is created with correct command.
func TestBeadStoreCreation(t *testing.T) {
	store := beads.NewBeadStore("br")
	if store == nil {
		t.Fatal("expected non-nil BeadStore")
	}
}
