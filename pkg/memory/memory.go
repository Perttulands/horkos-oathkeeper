package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CommitmentEntry holds the commitment data to be written to a memory file.
type CommitmentEntry struct {
	ID         string
	Text       string
	Category   string
	Source     string
	Status     string
	DetectedAt time.Time
	ExpiresAt  *time.Time
	BackedBy   []string
}

// Writer writes commitment data to markdown files in a memory directory.
type Writer struct {
	dir string
}

// NewWriter creates a Writer that writes to the given directory.
func NewWriter(dir string) *Writer {
	return &Writer{dir: dir}
}

// FilePath returns the full path for a commitment's memory file.
func (w *Writer) FilePath(id string) string {
	return filepath.Join(w.dir, fmt.Sprintf("oathkeeper-%s.md", id))
}

// WriteCommitment writes a commitment entry to a markdown file.
// If the file already exists (same ID), it is overwritten.
func (w *Writer) WriteCommitment(e CommitmentEntry) error {
	if err := os.MkdirAll(w.dir, 0755); err != nil {
		return fmt.Errorf("create memory directory: %w", err)
	}

	content := formatEntry(e)
	path := w.FilePath(e.ID)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write memory file: %w", err)
	}

	return nil
}

// RemoveCommitment removes a commitment's memory file. Returns nil if the file does not exist.
func (w *Writer) RemoveCommitment(id string) error {
	path := w.FilePath(id)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove memory file: %w", err)
	}
	return nil
}

func formatEntry(e CommitmentEntry) string {
	var b strings.Builder

	fmt.Fprintln(&b, "# Oathkeeper Commitment")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "**ID:** %s\n", e.ID)
	fmt.Fprintf(&b, "**Text:** %s\n", e.Text)
	fmt.Fprintf(&b, "**Category:** %s\n", e.Category)
	fmt.Fprintf(&b, "**Source:** %s\n", e.Source)
	fmt.Fprintf(&b, "**Status:** %s\n", e.Status)
	fmt.Fprintf(&b, "**Detected:** %s\n", e.DetectedAt.Format(time.RFC3339))

	if e.ExpiresAt != nil {
		fmt.Fprintf(&b, "**Expires:** %s\n", e.ExpiresAt.Format(time.RFC3339))
	}

	if len(e.BackedBy) > 0 {
		fmt.Fprintf(&b, "**Backed by:** %s\n", strings.Join(e.BackedBy, ", "))
	} else {
		fmt.Fprintln(&b, "**Backed by:** (none)")
	}

	return b.String()
}
