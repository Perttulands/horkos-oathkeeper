package hooks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestWebhookFiresUnbackedEvent(t *testing.T) {
	var received WebhookPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wh := NewWebhook(srv.URL)
	err := wh.NotifyUnbacked("bd-123", "I'll check in 5 minutes", "temporal")
	if err != nil {
		t.Fatalf("NotifyUnbacked failed: %v", err)
	}

	if received.Event != EventUnbacked {
		t.Fatalf("expected event %q, got %q", EventUnbacked, received.Event)
	}
	if received.BeadID != "bd-123" {
		t.Fatalf("expected bead_id bd-123, got %q", received.BeadID)
	}
	if received.Text != "I'll check in 5 minutes" {
		t.Fatalf("expected text, got %q", received.Text)
	}
	if received.Category != "temporal" {
		t.Fatalf("expected category temporal, got %q", received.Category)
	}
}

func TestWebhookFiresResolvedEvent(t *testing.T) {
	var received WebhookPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wh := NewWebhook(srv.URL)
	err := wh.NotifyResolved("bd-456", "checked and confirmed")
	if err != nil {
		t.Fatalf("NotifyResolved failed: %v", err)
	}

	if received.Event != EventResolved {
		t.Fatalf("expected event %q, got %q", EventResolved, received.Event)
	}
	if received.BeadID != "bd-456" {
		t.Fatalf("expected bead_id bd-456, got %q", received.BeadID)
	}
	if received.Evidence != "checked and confirmed" {
		t.Fatalf("expected evidence, got %q", received.Evidence)
	}
	if received.ResolvedAt == "" {
		t.Fatal("expected resolved_at timestamp for resolved event")
	}
}

func TestWebhookRetriesOnServerError(t *testing.T) {
	var attempts int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt64(&attempts, 1)
		if count < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wh := NewWebhook(srv.URL)
	wh.SetBaseDelay(1 * time.Millisecond) // fast retries for test

	err := wh.NotifyUnbacked("bd-789", "test retry", "temporal")
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}

	if atomic.LoadInt64(&attempts) != 3 {
		t.Fatalf("expected 3 attempts, got %d", atomic.LoadInt64(&attempts))
	}
}

func TestWebhookGivesUpAfterMaxRetries(t *testing.T) {
	var attempts int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	wh := NewWebhook(srv.URL)
	wh.SetBaseDelay(1 * time.Millisecond)

	err := wh.NotifyUnbacked("bd-fail", "always fails", "temporal")
	if err == nil {
		t.Fatal("expected error after max retries, got nil")
	}

	if atomic.LoadInt64(&attempts) != 3 {
		t.Fatalf("expected 3 attempts, got %d", atomic.LoadInt64(&attempts))
	}
}

func TestWebhookNoRetryOnClientError(t *testing.T) {
	var attempts int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	wh := NewWebhook(srv.URL)
	wh.SetBaseDelay(1 * time.Millisecond)

	err := wh.NotifyUnbacked("bd-client-err", "client error", "temporal")
	if err == nil {
		t.Fatal("expected error on client error, got nil")
	}

	if atomic.LoadInt64(&attempts) != 1 {
		t.Fatalf("expected 1 attempt (no retry on 4xx), got %d", atomic.LoadInt64(&attempts))
	}
}
