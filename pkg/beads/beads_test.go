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
		t.Fatal("expected error for missing bd command")
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

	return NewBeadStore(wrapperPath)
}

// --- Unit tests for pure functions (no bd dependency) ---

func TestNewBeadStoreDefaultCommand(t *testing.T) {
	store := NewBeadStore("")
	if store.command != "bd" {
		t.Errorf("expected default command 'bd', got %q", store.command)
	}

	store2 := NewBeadStore("  ")
	if store2.command != "bd" {
		t.Errorf("expected default command 'bd' for whitespace input, got %q", store2.command)
	}
}

func TestParseBeadListJSON_Array(t *testing.T) {
	payload := `[{"id":"abc","title":"test","status":"open","labels":["oathkeeper"],"created_at":"2026-02-13T14:30:00Z"}]`
	beads, err := parseBeadListJSON([]byte(payload))
	if err != nil {
		t.Fatalf("parseBeadListJSON: %v", err)
	}
	if len(beads) != 1 {
		t.Fatalf("expected 1 bead, got %d", len(beads))
	}
	if beads[0].ID != "abc" {
		t.Errorf("expected ID abc, got %q", beads[0].ID)
	}
	if beads[0].Status != "open" {
		t.Errorf("expected status open, got %q", beads[0].Status)
	}
}

func TestParseBeadListJSON_Single(t *testing.T) {
	payload := `{"id":"single","title":"one","status":"closed","labels":["oathkeeper"],"created_at":"2026-02-13T14:30:00Z","closed_at":"2026-02-13T15:00:00Z","close_reason":"done"}`
	beads, err := parseBeadListJSON([]byte(payload))
	if err != nil {
		t.Fatalf("parseBeadListJSON: %v", err)
	}
	if len(beads) != 1 {
		t.Fatalf("expected 1 bead, got %d", len(beads))
	}
	if beads[0].CloseReason != "done" {
		t.Errorf("expected close reason 'done', got %q", beads[0].CloseReason)
	}
}

func TestParseBeadListJSON_Empty(t *testing.T) {
	_, err := parseBeadListJSON([]byte(""))
	if err == nil {
		t.Fatal("expected error for empty payload")
	}
}

func TestParseBeadListJSON_InvalidJSON(t *testing.T) {
	_, err := parseBeadListJSON([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseBeadListJSON_TagsOverLabels(t *testing.T) {
	// When both tags and labels are present, tags should be preferred
	payload := `[{"id":"x","title":"t","status":"open","tags":["a","b"],"labels":["c"],"created_at":"2026-02-13T14:30:00Z"}]`
	beads, err := parseBeadListJSON([]byte(payload))
	if err != nil {
		t.Fatalf("parseBeadListJSON: %v", err)
	}
	if len(beads[0].Tags) != 2 || beads[0].Tags[0] != "a" {
		t.Errorf("expected tags [a b], got %v", beads[0].Tags)
	}
}

func TestParseBeadListJSON_LabelsWhenNoTags(t *testing.T) {
	payload := `[{"id":"x","title":"t","status":"open","labels":["oathkeeper","temporal"],"created_at":"2026-02-13T14:30:00Z"}]`
	beads, err := parseBeadListJSON([]byte(payload))
	if err != nil {
		t.Fatalf("parseBeadListJSON: %v", err)
	}
	if len(beads[0].Tags) != 2 || beads[0].Tags[0] != "oathkeeper" {
		t.Errorf("expected labels as tags, got %v", beads[0].Tags)
	}
}

func TestSessionTag(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"main", "session-main"},
		{"Main Session", "session-main-session"},
		{"", ""},
		{"  ", ""},
		{"abc-123_def", "session-abc-123_def"},
		{"a@b.c", "session-a-b-c"},
		{"---", ""},
	}
	for _, tt := range tests {
		got := sessionTag(tt.input)
		if got != tt.want {
			t.Errorf("sessionTag(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestUniqueStrings(t *testing.T) {
	got := uniqueStrings([]string{"a", "b", "a", "c", "b", ""})
	if len(got) != 3 {
		t.Fatalf("expected 3 unique strings, got %v", got)
	}
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("expected [a b c], got %v", got)
	}
}

func TestUniqueStringsEmpty(t *testing.T) {
	got := uniqueStrings(nil)
	if len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
}

func TestCreateTags(t *testing.T) {
	tags := createTags(CommitmentInfo{
		Category:   "temporal",
		SessionKey: "main",
	})
	if !hasTag(tags, "oathkeeper") {
		t.Error("missing oathkeeper tag")
	}
	if !hasTag(tags, "temporal") {
		t.Error("missing temporal tag")
	}
	if !hasTag(tags, "session-main") {
		t.Error("missing session-main tag")
	}
}

func TestCreateTagsWithExplicitTags(t *testing.T) {
	tags := createTags(CommitmentInfo{
		Category:   "temporal",
		Tags:       []string{"incident", "team_ops", "incident", " "},
		SessionKey: "main",
	})

	if !hasTag(tags, "oathkeeper") {
		t.Error("missing oathkeeper tag")
	}
	if !hasTag(tags, "temporal") {
		t.Error("missing temporal tag")
	}
	if !hasTag(tags, "incident") {
		t.Error("missing explicit incident tag")
	}
	if !hasTag(tags, "team_ops") {
		t.Error("missing explicit team_ops tag")
	}

	incidentCount := 0
	for _, tag := range tags {
		if tag == "incident" {
			incidentCount++
		}
	}
	if incidentCount != 1 {
		t.Fatalf("expected incident tag once, got %d in %v", incidentCount, tags)
	}
}

func TestCreateTagsNoSession(t *testing.T) {
	tags := createTags(CommitmentInfo{Category: "followup"})
	if !hasTag(tags, "oathkeeper") {
		t.Error("missing oathkeeper tag")
	}
	if !hasTag(tags, "followup") {
		t.Error("missing followup tag")
	}
	// Should not contain session tag
	for _, tag := range tags {
		if strings.HasPrefix(tag, "session-") {
			t.Errorf("unexpected session tag: %s", tag)
		}
	}
}

func TestBuildListArgs(t *testing.T) {
	store := NewBeadStore("bd")

	args := store.buildListArgs(Filter{Status: "open"})
	assertContains(t, args, "--label")
	assertContains(t, args, "oathkeeper")
	assertContains(t, args, "--json")
	assertContains(t, args, "--status")
	assertContains(t, args, "open")
}

func TestBuildListArgsClosed(t *testing.T) {
	store := NewBeadStore("bd")

	args := store.buildListArgs(Filter{Status: "closed"})
	assertContains(t, args, "--all")
	assertContains(t, args, "--status")
	assertContains(t, args, "closed")
}

func TestBuildListArgsWithCategory(t *testing.T) {
	store := NewBeadStore("bd")

	args := store.buildListArgs(Filter{Category: "temporal"})
	// Should have two --label flags: one for oathkeeper, one for temporal
	labelCount := 0
	for _, a := range args {
		if a == "--label" {
			labelCount++
		}
	}
	if labelCount != 2 {
		t.Errorf("expected 2 --label flags, got %d in %v", labelCount, args)
	}
}

func TestBuildCreateArgs(t *testing.T) {
	args := buildCreateArgs("oathkeeper: test", []string{"oathkeeper", "temporal"})
	assertContains(t, args, "--labels")
	assertContains(t, args, "oathkeeper,temporal")
	assertContains(t, args, "--silent")
	assertContains(t, args, "--priority")
}

func TestCloseEmptyID(t *testing.T) {
	store := NewBeadStore("bd")
	err := store.Close("", "reason")
	if err == nil {
		t.Fatal("expected error for empty bead ID")
	}
}

func TestGetEmptyID(t *testing.T) {
	store := NewBeadStore("bd")
	_, err := store.Get("")
	if err == nil {
		t.Fatal("expected error for empty bead ID")
	}
}

func TestResolveEmptyID(t *testing.T) {
	store := NewBeadStore("bd")
	err := store.Resolve("", "evidence")
	if err == nil {
		t.Fatal("expected error for empty bead ID")
	}
}

func TestResolveEmptyEvidence(t *testing.T) {
	store := NewBeadStore("bd")
	err := store.Resolve("some-id", "")
	if err == nil {
		t.Fatal("expected error for empty evidence")
	}
}

func TestContainsResolutionIndicator(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"I checked the logs", true},
		{"Done with the deployment", true},
		{"Completed the review", true},
		{"Here are the results of my investigation", true},
		{"Can you check on this later?", false},
		{"", false},
		{"The weather is nice", false},
	}
	for _, tt := range tests {
		got := containsResolutionIndicator(tt.msg)
		if got != tt.want {
			t.Errorf("containsResolutionIndicator(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

func TestFilterBeads(t *testing.T) {
	now := time.Now()
	beads := []Bead{
		{ID: "old", Tags: []string{"temporal"}, CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "new", Tags: []string{"temporal"}, CreatedAt: now.Add(-1 * time.Minute)},
		{ID: "other", Tags: []string{"followup"}, CreatedAt: now.Add(-1 * time.Minute)},
	}

	// Filter by Since
	filtered := filterBeads(beads, Filter{Since: now.Add(-30 * time.Minute)})
	if len(filtered) != 2 {
		t.Fatalf("expected 2 beads after Since filter, got %d", len(filtered))
	}

	// Filter by Category
	filtered = filterBeads(beads, Filter{Category: "followup"})
	if len(filtered) != 1 || filtered[0].ID != "other" {
		t.Errorf("expected [other] after category filter, got %v", filtered)
	}

	// No filter
	filtered = filterBeads(beads, Filter{})
	if len(filtered) != 3 {
		t.Errorf("expected all 3 beads with empty filter, got %d", len(filtered))
	}
}

func TestAutoResolveEmptySessionKey(t *testing.T) {
	store := NewBeadStore("definitely-not-here")
	resolved, err := store.AutoResolve("", "I checked")
	if err != nil {
		t.Fatalf("AutoResolve with empty session: %v", err)
	}
	if len(resolved) != 0 {
		t.Errorf("expected 0 resolved, got %v", resolved)
	}
}

func TestAutoResolveNoIndicator(t *testing.T) {
	store := NewBeadStore("definitely-not-here")
	resolved, err := store.AutoResolve("main", "nothing happened")
	if err != nil {
		t.Fatalf("AutoResolve with no indicator: %v", err)
	}
	if len(resolved) != 0 {
		t.Errorf("expected 0 resolved, got %v", resolved)
	}
}

func assertContains(t *testing.T, args []string, want string) {
	t.Helper()
	for _, a := range args {
		if a == want {
			return
		}
	}
	t.Errorf("args %v does not contain %q", args, want)
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
