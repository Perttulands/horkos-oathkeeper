package beads

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateReturnsCommandUnavailableWhenBRMissing(t *testing.T) {
	store := NewBeadStore("definitely-missing-br-command")

	_, err := store.Create(CommitmentInfo{
		Text:       "I'll check that",
		Category:   "temporal",
		SessionKey: "main",
		DetectedAt: time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(5 * time.Minute),
	})
	if err == nil {
		t.Fatal("expected error for missing br command")
	}
	if !errors.Is(err, ErrCommandUnavailable) {
		t.Fatalf("expected ErrCommandUnavailable, got: %v", err)
	}
}

func TestBeadStoreLifecycleWithRealBR(t *testing.T) {
	store := newTestBeadStore(t)

	id, err := store.Create(CommitmentInfo{
		Text:       "I'll report back in 10 minutes",
		Category:   "temporal",
		SessionKey: "main",
		DetectedAt: time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if id == "" {
		t.Fatal("Create returned empty bead ID")
	}

	bead, err := store.Get(id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if bead.ID != id {
		t.Fatalf("Get returned bead ID %q, want %q", bead.ID, id)
	}
	if bead.Status != "open" {
		t.Fatalf("Get returned status %q, want open", bead.Status)
	}
	if !strings.Contains(bead.Title, "oathkeeper: ") {
		t.Fatalf("Get returned title %q, want oathkeeper prefix", bead.Title)
	}
	if !hasTag(bead.Tags, "oathkeeper") {
		t.Fatalf("Get returned tags %v, missing oathkeeper", bead.Tags)
	}
	if !hasTag(bead.Tags, "temporal") {
		t.Fatalf("Get returned tags %v, missing temporal", bead.Tags)
	}

	open, err := store.List(Filter{Status: "open"})
	if err != nil {
		t.Fatalf("List(open) failed: %v", err)
	}
	if !containsBeadID(open, id) {
		t.Fatalf("List(open) did not include created bead %q", id)
	}

	temporal, err := store.List(Filter{Status: "open", Category: "temporal"})
	if err != nil {
		t.Fatalf("List(open, temporal) failed: %v", err)
	}
	if !containsBeadID(temporal, id) {
		t.Fatalf("List(open, temporal) did not include created bead %q", id)
	}

	if err := store.Close(id, "completed"); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	closed, err := store.Get(id)
	if err != nil {
		t.Fatalf("Get after Close failed: %v", err)
	}
	if closed.Status != "closed" {
		t.Fatalf("Get after Close returned status %q, want closed", closed.Status)
	}
	if closed.ClosedAt.IsZero() {
		t.Fatal("Get after Close returned zero ClosedAt")
	}

	stillOpen, err := store.List(Filter{Status: "open"})
	if err != nil {
		t.Fatalf("List(open) after Close failed: %v", err)
	}
	if containsBeadID(stillOpen, id) {
		t.Fatalf("List(open) still includes closed bead %q", id)
	}
}

func TestListAppliesSinceFilter(t *testing.T) {
	store := newTestBeadStore(t)

	firstID, err := store.Create(CommitmentInfo{
		Text:       "I'll check the first item",
		Category:   "followup",
		SessionKey: "main",
		DetectedAt: time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Create first bead failed: %v", err)
	}

	time.Sleep(20 * time.Millisecond)
	since := time.Now().UTC()
	time.Sleep(20 * time.Millisecond)

	secondID, err := store.Create(CommitmentInfo{
		Text:       "I'll check the second item",
		Category:   "followup",
		SessionKey: "main",
		DetectedAt: time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Create second bead failed: %v", err)
	}

	filtered, err := store.List(Filter{
		Status:   "open",
		Category: "followup",
		Since:    since,
	})
	if err != nil {
		t.Fatalf("List with Since failed: %v", err)
	}

	if containsBeadID(filtered, firstID) {
		t.Fatalf("List with Since unexpectedly included first bead %q", firstID)
	}
	if !containsBeadID(filtered, secondID) {
		t.Fatalf("List with Since did not include second bead %q", secondID)
	}
}

func newTestBeadStore(t *testing.T) *BeadStore {
	t.Helper()

	brPath, err := exec.LookPath("br")
	if err != nil {
		t.Skip("br not in PATH")
	}

	workspace := t.TempDir()
	beadsDir := filepath.Join(workspace, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("create beads dir: %v", err)
	}

	dbPath := filepath.Join(beadsDir, "beads.db")
	wrapperPath := filepath.Join(workspace, "br-wrapper.sh")
	wrapper := "#!/bin/sh\nBR=\"" + brPath + "\"\nDB=\"" + dbPath + "\"\nexec \"$BR\" --db \"$DB\" \"$@\"\n"
	if err := os.WriteFile(wrapperPath, []byte(wrapper), 0o755); err != nil {
		t.Fatalf("write wrapper script: %v", err)
	}

	return NewBeadStore(wrapperPath)
}

func hasTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}

func containsBeadID(beads []Bead, id string) bool {
	for _, bead := range beads {
		if bead.ID == id {
			return true
		}
	}
	return false
}
