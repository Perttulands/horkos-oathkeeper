package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tempDB(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q): %v", dbPath, err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func sampleCommitment(id string) Commitment {
	now := time.Now().Truncate(time.Second)
	expires := now.Add(24 * time.Hour)
	return Commitment{
		ID:         id,
		DetectedAt: now,
		Source:     "athena-01",
		MessageID:  "msg-" + id,
		Text:       "I'll check back in 5 minutes",
		Category:   CategoryTemporal,
		BackedBy:   []string{},
		Status:     StatusUnverified,
		ExpiresAt:  &expires,
		AlertCount: 0,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func TestOpenCreatesDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sub", "commitments.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("database file not created: %v", err)
	}
}

func TestInsertAndGet(t *testing.T) {
	s := tempDB(t)
	c := sampleCommitment("abc123")

	if err := s.Insert(c); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := s.Get("abc123")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.ID != "abc123" {
		t.Errorf("ID = %q, want %q", got.ID, "abc123")
	}
	if got.Text != c.Text {
		t.Errorf("Text = %q, want %q", got.Text, c.Text)
	}
	if got.Category != CategoryTemporal {
		t.Errorf("Category = %q, want %q", got.Category, CategoryTemporal)
	}
	if got.Status != StatusUnverified {
		t.Errorf("Status = %q, want %q", got.Status, StatusUnverified)
	}
	if got.Source != "athena-01" {
		t.Errorf("Source = %q, want %q", got.Source, "athena-01")
	}
	if got.ExpiresAt == nil {
		t.Fatal("ExpiresAt is nil, want non-nil")
	}
	if len(got.BackedBy) != 0 {
		t.Errorf("BackedBy = %v, want empty", got.BackedBy)
	}
}

func TestGetNotFound(t *testing.T) {
	s := tempDB(t)
	_, err := s.Get("nonexistent")
	if err != ErrNotFound {
		t.Errorf("Get(nonexistent) = %v, want ErrNotFound", err)
	}
}

func TestInsertDuplicate(t *testing.T) {
	s := tempDB(t)
	c := sampleCommitment("dup1")
	if err := s.Insert(c); err != nil {
		t.Fatalf("first Insert: %v", err)
	}
	err := s.Insert(c)
	if err == nil {
		t.Fatal("second Insert: want error, got nil")
	}
}

func TestListAll(t *testing.T) {
	s := tempDB(t)
	s.Insert(sampleCommitment("a1"))
	s.Insert(sampleCommitment("a2"))
	s.Insert(sampleCommitment("a3"))

	results, err := s.List(ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("List() returned %d items, want 3", len(results))
	}
}

func TestListFilterByStatus(t *testing.T) {
	s := tempDB(t)

	c1 := sampleCommitment("s1")
	c1.Status = StatusUnverified
	s.Insert(c1)

	c2 := sampleCommitment("s2")
	c2.Status = StatusBacked
	s.Insert(c2)

	c3 := sampleCommitment("s3")
	c3.Status = StatusAlerted
	s.Insert(c3)

	results, err := s.List(ListFilter{Status: StatusBacked})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("List(status=backed) returned %d items, want 1", len(results))
	}
	if results[0].ID != "s2" {
		t.Errorf("ID = %q, want %q", results[0].ID, "s2")
	}
}

func TestListFilterByCategory(t *testing.T) {
	s := tempDB(t)

	c1 := sampleCommitment("cat1")
	c1.Category = CategoryTemporal
	s.Insert(c1)

	c2 := sampleCommitment("cat2")
	c2.Category = CategoryConditional
	s.Insert(c2)

	results, err := s.List(ListFilter{Category: CategoryConditional})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("List(category=conditional) returned %d items, want 1", len(results))
	}
	if results[0].ID != "cat2" {
		t.Errorf("ID = %q, want %q", results[0].ID, "cat2")
	}
}

func TestListFilterBySince(t *testing.T) {
	s := tempDB(t)

	old := sampleCommitment("old1")
	old.DetectedAt = time.Now().Add(-48 * time.Hour)
	s.Insert(old)

	recent := sampleCommitment("new1")
	recent.DetectedAt = time.Now().Add(-1 * time.Hour)
	s.Insert(recent)

	since := 24 * time.Hour
	results, err := s.List(ListFilter{Since: &since})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("List(since=24h) returned %d items, want 1", len(results))
	}
	if results[0].ID != "new1" {
		t.Errorf("ID = %q, want %q", results[0].ID, "new1")
	}
}

func TestListCombinedFilters(t *testing.T) {
	s := tempDB(t)

	c1 := sampleCommitment("cf1")
	c1.Status = StatusAlerted
	c1.Category = CategoryTemporal
	s.Insert(c1)

	c2 := sampleCommitment("cf2")
	c2.Status = StatusAlerted
	c2.Category = CategoryConditional
	s.Insert(c2)

	c3 := sampleCommitment("cf3")
	c3.Status = StatusBacked
	c3.Category = CategoryTemporal
	s.Insert(c3)

	results, err := s.List(ListFilter{Status: StatusAlerted, Category: CategoryTemporal})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("List(status=alerted,category=temporal) returned %d items, want 1", len(results))
	}
	if results[0].ID != "cf1" {
		t.Errorf("ID = %q, want %q", results[0].ID, "cf1")
	}
}

func TestUpdateStatus(t *testing.T) {
	s := tempDB(t)
	c := sampleCommitment("upd1")
	s.Insert(c)

	now := time.Now().Truncate(time.Second)
	err := s.UpdateStatus("upd1", StatusBacked, []string{"cron:abc123"}, now)
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	got, _ := s.Get("upd1")
	if got.Status != StatusBacked {
		t.Errorf("Status = %q, want %q", got.Status, StatusBacked)
	}
	if len(got.BackedBy) != 1 || got.BackedBy[0] != "cron:abc123" {
		t.Errorf("BackedBy = %v, want [cron:abc123]", got.BackedBy)
	}
	if got.LastChecked == nil {
		t.Fatal("LastChecked is nil after update")
	}
}

func TestUpdateStatusNotFound(t *testing.T) {
	s := tempDB(t)
	err := s.UpdateStatus("nonexistent", StatusBacked, nil, time.Now())
	if err != ErrNotFound {
		t.Errorf("UpdateStatus(nonexistent) = %v, want ErrNotFound", err)
	}
}

func TestIncrementAlertCount(t *testing.T) {
	s := tempDB(t)
	c := sampleCommitment("alert1")
	s.Insert(c)

	if err := s.IncrementAlertCount("alert1"); err != nil {
		t.Fatalf("IncrementAlertCount: %v", err)
	}

	got, _ := s.Get("alert1")
	if got.AlertCount != 1 {
		t.Errorf("AlertCount = %d, want 1", got.AlertCount)
	}

	s.IncrementAlertCount("alert1")
	got, _ = s.Get("alert1")
	if got.AlertCount != 2 {
		t.Errorf("AlertCount = %d, want 2", got.AlertCount)
	}
}

func TestNilExpiresAt(t *testing.T) {
	s := tempDB(t)
	c := sampleCommitment("noexp")
	c.ExpiresAt = nil
	s.Insert(c)

	got, _ := s.Get("noexp")
	if got.ExpiresAt != nil {
		t.Errorf("ExpiresAt = %v, want nil", got.ExpiresAt)
	}
}

func TestBackedByJSONRoundTrip(t *testing.T) {
	s := tempDB(t)
	c := sampleCommitment("backed1")
	c.BackedBy = []string{"cron:abc", "bead:xyz", "file:/tmp/state.json"}
	s.Insert(c)

	got, _ := s.Get("backed1")
	if len(got.BackedBy) != 3 {
		t.Fatalf("BackedBy len = %d, want 3", len(got.BackedBy))
	}
	if got.BackedBy[0] != "cron:abc" {
		t.Errorf("BackedBy[0] = %q, want %q", got.BackedBy[0], "cron:abc")
	}
	if got.BackedBy[2] != "file:/tmp/state.json" {
		t.Errorf("BackedBy[2] = %q, want %q", got.BackedBy[2], "file:/tmp/state.json")
	}
}

func TestListOrderByDetectedAtDesc(t *testing.T) {
	s := tempDB(t)

	c1 := sampleCommitment("ord1")
	c1.DetectedAt = time.Now().Add(-3 * time.Hour)
	s.Insert(c1)

	c2 := sampleCommitment("ord2")
	c2.DetectedAt = time.Now().Add(-1 * time.Hour)
	s.Insert(c2)

	c3 := sampleCommitment("ord3")
	c3.DetectedAt = time.Now().Add(-2 * time.Hour)
	s.Insert(c3)

	results, _ := s.List(ListFilter{})
	if results[0].ID != "ord2" {
		t.Errorf("first result ID = %q, want %q (most recent)", results[0].ID, "ord2")
	}
	if results[2].ID != "ord1" {
		t.Errorf("last result ID = %q, want %q (oldest)", results[2].ID, "ord1")
	}
}
