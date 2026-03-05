package ingest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTranscriptPollerSkipsExistingThenReadsAppend(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "ses-1")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	file := filepath.Join(sessionDir, "transcript.jsonl")
	if err := os.WriteFile(file, []byte("{\"role\":\"assistant\",\"content\":\"I'll check back in 5 minutes\"}\n"), 0o644); err != nil {
		t.Fatalf("write initial: %v", err)
	}

	var got []Message
	p := NewTranscriptPoller(root, 0, func(m Message) error {
		got = append(got, m)
		return nil
	})

	if err := p.scanOnce(); err != nil {
		t.Fatalf("scanOnce initial: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no replay of historical content, got %d", len(got))
	}

	f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open append: %v", err)
	}
	if _, err := f.WriteString("{\"role\":\"assistant\",\"content\":\"I'll report back\"}\n"); err != nil {
		_ = f.Close()
		t.Fatalf("append: %v", err)
	}
	_ = f.Close()

	if err := p.scanOnce(); err != nil {
		t.Fatalf("scanOnce append: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 appended message, got %d", len(got))
	}
	if got[0].SessionKey != "ses-1" {
		t.Fatalf("expected session ses-1, got %q", got[0].SessionKey)
	}
	if got[0].Text != "I'll report back" {
		t.Fatalf("unexpected text: %q", got[0].Text)
	}
}

func TestTranscriptPollerParsesOpenClawNested(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "ses-2")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	file := filepath.Join(sessionDir, "transcript.jsonl")
	if err := os.WriteFile(file, []byte{}, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var got []Message
	p := NewTranscriptPoller(root, 0, func(m Message) error {
		got = append(got, m)
		return nil
	})

	if err := p.scanOnce(); err != nil {
		t.Fatalf("scanOnce initial: %v", err)
	}

	content := "{\"id\":\"msg-1\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"I'll monitor this\"}]}}\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	if err := p.scanOnce(); err != nil {
		t.Fatalf("scanOnce nested: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}
	if got[0].MessageID != "msg-1" {
		t.Fatalf("expected msg id msg-1, got %q", got[0].MessageID)
	}
}

func TestTranscriptPollerDedupesReplayAfterTruncate(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "ses-3")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	file := filepath.Join(sessionDir, "transcript.jsonl")
	if err := os.WriteFile(file, []byte{}, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var got []Message
	p := NewTranscriptPoller(root, 0, func(m Message) error {
		got = append(got, m)
		return nil
	})

	if err := p.scanOnce(); err != nil {
		t.Fatalf("scanOnce initial: %v", err)
	}

	line := "{\"role\":\"assistant\",\"content\":\"I need to check this\"}\n"
	if err := os.WriteFile(file, []byte(line), 0o644); err != nil {
		t.Fatalf("write line: %v", err)
	}
	if err := p.scanOnce(); err != nil {
		t.Fatalf("scanOnce first line: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}

	if err := os.WriteFile(file, []byte(line), 0o644); err != nil {
		t.Fatalf("rewrite line: %v", err)
	}
	if err := p.scanOnce(); err != nil {
		t.Fatalf("scanOnce replay: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected replay to be deduped; got %d", len(got))
	}
}
