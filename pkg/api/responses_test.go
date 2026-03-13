package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteErrorWithDetailTrimsFields(t *testing.T) {
	w := httptest.NewRecorder()

	writeErrorWithDetail(w, http.StatusBadRequest, "bad request", "  detail here  ", "  hint here  ")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected application/json content type, got %q", got)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Detail != "detail here" {
		t.Fatalf("expected trimmed detail, got %q", resp.Detail)
	}
	if resp.Hint != "hint here" {
		t.Fatalf("expected trimmed hint, got %q", resp.Hint)
	}
}
