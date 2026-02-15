package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewWriter(t *testing.T) {
	w := NewWriter("/tmp/test-memory")
	if w.dir != "/tmp/test-memory" {
		t.Errorf("dir = %q, want %q", w.dir, "/tmp/test-memory")
	}
}

func TestWriteCommitment(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)

	detected := time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC)
	expires := time.Date(2026, 2, 13, 14, 35, 0, 0, time.UTC)

	err := w.WriteCommitment(CommitmentEntry{
		ID:         "abc123",
		Text:       "I'll check back in 5 minutes",
		Category:   "temporal",
		Source:     "athena-01",
		Status:     "unverified",
		DetectedAt: detected,
		ExpiresAt:  &expires,
		BackedBy:   []string{},
	})
	if err != nil {
		t.Fatalf("WriteCommitment() error = %v", err)
	}

	// Verify file was created
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}

	// Verify filename format: oathkeeper-<id>.md
	name := entries[0].Name()
	if !strings.HasPrefix(name, "oathkeeper-abc123") {
		t.Errorf("filename = %q, want prefix oathkeeper-abc123", name)
	}
	if !strings.HasSuffix(name, ".md") {
		t.Errorf("filename = %q, want .md suffix", name)
	}

	// Verify content
	content, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	s := string(content)

	if !strings.Contains(s, "abc123") {
		t.Error("content missing commitment ID")
	}
	if !strings.Contains(s, "I'll check back in 5 minutes") {
		t.Error("content missing commitment text")
	}
	if !strings.Contains(s, "temporal") {
		t.Error("content missing category")
	}
	if !strings.Contains(s, "athena-01") {
		t.Error("content missing source")
	}
	if !strings.Contains(s, "unverified") {
		t.Error("content missing status")
	}
	if !strings.Contains(s, "2026-02-13T14:30:00Z") {
		t.Error("content missing detected_at timestamp")
	}
	if !strings.Contains(s, "2026-02-13T14:35:00Z") {
		t.Error("content missing expires_at timestamp")
	}
}

func TestWriteCommitmentNoExpiry(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)

	detected := time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC)

	err := w.WriteCommitment(CommitmentEntry{
		ID:         "def456",
		Text:       "I'll follow up on this",
		Category:   "followup",
		Source:     "athena-02",
		Status:     "alerted",
		DetectedAt: detected,
		ExpiresAt:  nil,
		BackedBy:   []string{},
	})
	if err != nil {
		t.Fatalf("WriteCommitment() error = %v", err)
	}

	entries, _ := os.ReadDir(dir)
	content, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	s := string(content)

	if strings.Contains(s, "Expires:") {
		t.Error("content should not contain Expires when nil")
	}
}

func TestWriteCommitmentWithMechanisms(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)

	detected := time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC)

	err := w.WriteCommitment(CommitmentEntry{
		ID:         "ghi789",
		Text:       "I'll notify you at 3pm",
		Category:   "scheduled",
		Source:     "athena-01",
		Status:     "backed",
		DetectedAt: detected,
		BackedBy:   []string{"cron:abc123", "bead:build-watcher"},
	})
	if err != nil {
		t.Fatalf("WriteCommitment() error = %v", err)
	}

	entries, _ := os.ReadDir(dir)
	content, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	s := string(content)

	if !strings.Contains(s, "cron:abc123") {
		t.Error("content missing first mechanism")
	}
	if !strings.Contains(s, "bead:build-watcher") {
		t.Error("content missing second mechanism")
	}
}

func TestWriteCommitmentCreatesDirectory(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "nested", "memory")
	w := NewWriter(dir)

	detected := time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC)

	err := w.WriteCommitment(CommitmentEntry{
		ID:         "jkl012",
		Text:       "I'll check on this",
		Category:   "temporal",
		Source:     "athena-01",
		Status:     "unverified",
		DetectedAt: detected,
		BackedBy:   []string{},
	})
	if err != nil {
		t.Fatalf("WriteCommitment() error = %v", err)
	}

	// Verify directory and file exist
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}
}

func TestWriteCommitmentInvalidDirectory(t *testing.T) {
	w := NewWriter("/dev/null/impossible")

	err := w.WriteCommitment(CommitmentEntry{
		ID:         "err123",
		Text:       "test",
		Category:   "temporal",
		Source:     "test",
		Status:     "unverified",
		DetectedAt: time.Now(),
		BackedBy:   []string{},
	})
	if err == nil {
		t.Error("expected error for invalid directory")
	}
}

func TestWriteMultipleCommitments(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)

	detected := time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC)

	for _, id := range []string{"aaa", "bbb", "ccc"} {
		err := w.WriteCommitment(CommitmentEntry{
			ID:         id,
			Text:       "commitment " + id,
			Category:   "temporal",
			Source:     "athena-01",
			Status:     "unverified",
			DetectedAt: detected,
			BackedBy:   []string{},
		})
		if err != nil {
			t.Fatalf("WriteCommitment(%s) error = %v", id, err)
		}
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Errorf("expected 3 files, got %d", len(entries))
	}
}

func TestFilePath(t *testing.T) {
	w := NewWriter("/tmp/memory")

	path := w.FilePath("abc123")
	expected := "/tmp/memory/oathkeeper-abc123.md"
	if path != expected {
		t.Errorf("FilePath() = %q, want %q", path, expected)
	}
}

func TestRemoveCommitment(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)

	detected := time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC)

	// Write a commitment
	err := w.WriteCommitment(CommitmentEntry{
		ID:         "rem123",
		Text:       "I'll check back",
		Category:   "temporal",
		Source:     "athena-01",
		Status:     "unverified",
		DetectedAt: detected,
		BackedBy:   []string{},
	})
	if err != nil {
		t.Fatalf("WriteCommitment() error = %v", err)
	}

	// Verify it exists
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}

	// Remove it
	err = w.RemoveCommitment("rem123")
	if err != nil {
		t.Fatalf("RemoveCommitment() error = %v", err)
	}

	// Verify it's gone
	entries, _ = os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("expected 0 files after remove, got %d", len(entries))
	}
}

func TestRemoveNonexistentCommitment(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)

	// Removing a nonexistent file should not error
	err := w.RemoveCommitment("nonexistent")
	if err != nil {
		t.Errorf("RemoveCommitment() error = %v, want nil", err)
	}
}

func TestContentIsMarkdownFormatted(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)

	detected := time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC)
	expires := time.Date(2026, 2, 13, 15, 30, 0, 0, time.UTC)

	err := w.WriteCommitment(CommitmentEntry{
		ID:         "md123",
		Text:       "I'll monitor the build",
		Category:   "followup",
		Source:     "athena-01",
		Status:     "alerted",
		DetectedAt: detected,
		ExpiresAt:  &expires,
		BackedBy:   []string{"bead:watcher"},
	})
	if err != nil {
		t.Fatalf("WriteCommitment() error = %v", err)
	}

	entries, _ := os.ReadDir(dir)
	content, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	s := string(content)

	// Check markdown header
	if !strings.HasPrefix(s, "# Oathkeeper Commitment") {
		t.Error("content should start with markdown header")
	}

	// Check structured fields
	if !strings.Contains(s, "**ID:**") {
		t.Error("content missing ID field label")
	}
	if !strings.Contains(s, "**Text:**") {
		t.Error("content missing Text field label")
	}
	if !strings.Contains(s, "**Category:**") {
		t.Error("content missing Category field label")
	}
	if !strings.Contains(s, "**Status:**") {
		t.Error("content missing Status field label")
	}
}

func TestUpdateCommitment(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)

	detected := time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC)

	// Write initial
	err := w.WriteCommitment(CommitmentEntry{
		ID:         "upd123",
		Text:       "I'll check back",
		Category:   "temporal",
		Source:     "athena-01",
		Status:     "unverified",
		DetectedAt: detected,
		BackedBy:   []string{},
	})
	if err != nil {
		t.Fatalf("initial WriteCommitment() error = %v", err)
	}

	// Update with new status
	err = w.WriteCommitment(CommitmentEntry{
		ID:         "upd123",
		Text:       "I'll check back",
		Category:   "temporal",
		Source:     "athena-01",
		Status:     "backed",
		DetectedAt: detected,
		BackedBy:   []string{"cron:xyz"},
	})
	if err != nil {
		t.Fatalf("update WriteCommitment() error = %v", err)
	}

	// Should still be 1 file (overwritten)
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected 1 file after update, got %d", len(entries))
	}

	// Should have updated content
	content, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	s := string(content)
	if !strings.Contains(s, "backed") {
		t.Error("content should contain updated status")
	}
	if !strings.Contains(s, "cron:xyz") {
		t.Error("content should contain mechanism")
	}
}

func TestFilePathSanitizesTraversal(t *testing.T) {
	w := NewWriter("/tmp/memory")

	// Attempting path traversal should be sanitized to just the base name
	path := w.FilePath("../../etc/passwd")
	if strings.Contains(path, "..") {
		t.Errorf("path traversal not sanitized: %s", path)
	}
	expected := "/tmp/memory/oathkeeper-passwd.md"
	if path != expected {
		t.Errorf("FilePath() = %q, want %q", path, expected)
	}
}

func TestFilePathSlashInID(t *testing.T) {
	w := NewWriter("/tmp/memory")
	path := w.FilePath("sub/dir/id")
	if strings.Contains(path, "sub/dir") {
		t.Errorf("slashes in ID should be sanitized: %s", path)
	}
}
