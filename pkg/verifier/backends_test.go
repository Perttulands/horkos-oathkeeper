package verifier

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileCheckerFindsRecentFiles(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.txt")
	newPath := filepath.Join(dir, "new.txt")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	if err := os.Chtimes(oldPath, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newPath, now.Add(-10*time.Second), now.Add(-10*time.Second)); err != nil {
		t.Fatal(err)
	}

	checker := NewFileChecker("state_files", []string{dir})
	mechanisms, err := checker.Check(now.Add(-1 * time.Minute))
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if len(mechanisms) != 1 {
		t.Fatalf("expected 1 mechanism, got %d (%v)", len(mechanisms), mechanisms)
	}
	if mechanisms[0] != "file:"+newPath {
		t.Fatalf("expected mechanism for new file, got %v", mechanisms)
	}
}

func TestBeadCheckerFindsRecentOpenBeads(t *testing.T) {
	now := time.Now().UTC()
	script := "#!/bin/sh\ncat <<'JSON'\n" +
		`[` +
		`{"id":"bd-old","created_at":"` + now.Add(-2*time.Hour).Format(time.RFC3339) + `"},` +
		`{"id":"bd-new","created_at":"` + now.Add(-5*time.Minute).Format(time.RFC3339) + `"}` +
		`]
JSON
`
	scriptPath := filepath.Join(t.TempDir(), "mock-beads.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	checker := NewBeadChecker(scriptPath)
	mechanisms, err := checker.Check(now.Add(-1 * time.Hour))
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if len(mechanisms) != 1 || mechanisms[0] != "bead:bd-new" {
		t.Fatalf("expected [bead:bd-new], got %v", mechanisms)
	}
}

func TestNewVerifierFromConfigIncludesBackends(t *testing.T) {
	dir := t.TempDir()
	fresh := filepath.Join(dir, "recent.log")
	if err := os.WriteFile(fresh, []byte("recent"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(CronAPIResponse{Crons: []CronJob{}})
	}))
	defer server.Close()

	v := NewVerifierFromConfig(Options{
		CronAPIURL:   server.URL,
		CronEndpoint: "/api/v1/crons",
		StateDirs:    []string{dir},
		MemoryDirs:   []string{dir},
	})

	result, err := v.Verify(time.Now().UTC().Add(-1 * time.Minute))
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if !containsString(result.CheckedSources, "crons") ||
		!containsString(result.CheckedSources, "state_files") ||
		!containsString(result.CheckedSources, "memory_files") {
		t.Fatalf("missing expected checked sources: %v", result.CheckedSources)
	}
	if !result.IsBacked {
		t.Fatalf("expected file backend to mark commitment as backed, got %v", result)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
