package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestScanFile_DetectsTemporalCommitment(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "transcript.jsonl", `{"role":"assistant","content":"I'll check back in 5 minutes"}
`)
	results, err := ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Category != "temporal" {
		t.Errorf("expected category temporal, got %s", results[0].Category)
	}
	if results[0].Text != "I'll check back in 5 minutes" {
		t.Errorf("unexpected text: %s", results[0].Text)
	}
}

func TestScanFile_DetectsConditionalCommitment(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "transcript.jsonl", `{"role":"assistant","content":"Once the build finishes, I'll notify you"}
`)
	results, err := ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Category != "conditional" {
		t.Errorf("expected category conditional, got %s", results[0].Category)
	}
}

func TestScanFile_IgnoresUserMessages(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "transcript.jsonl", `{"role":"user","content":"I'll check back in 5 minutes"}
`)
	results, err := ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for user message, got %d", len(results))
	}
}

func TestScanFile_IgnoresSystemDescriptions(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "transcript.jsonl", `{"role":"assistant","content":"The script will monitor the process"}
`)
	results, err := ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for system description, got %d", len(results))
	}
}

func TestScanFile_IgnoresPastTense(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "transcript.jsonl", `{"role":"assistant","content":"I created a cron job to handle it"}
`)
	results, err := ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for past tense, got %d", len(results))
	}
}

func TestScanFile_MultipleMessages(t *testing.T) {
	dir := t.TempDir()
	content := `{"role":"assistant","content":"I'll check back in 5 minutes"}
{"role":"user","content":"ok"}
{"role":"assistant","content":"Once the test passes, I'll deploy it"}
{"role":"assistant","content":"The script will handle the rest"}
`
	path := writeFile(t, dir, "transcript.jsonl", content)
	results, err := ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Category != "temporal" {
		t.Errorf("first result: expected temporal, got %s", results[0].Category)
	}
	if results[1].Category != "conditional" {
		t.Errorf("second result: expected conditional, got %s", results[1].Category)
	}
}

func TestScanFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "transcript.jsonl", "")
	results, err := ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for empty file, got %d", len(results))
	}
}

func TestScanFile_NonExistentFile(t *testing.T) {
	_, err := ScanFile("/nonexistent/path/transcript.jsonl")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestScanFile_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	content := `not valid json
{"role":"assistant","content":"I'll check back in 5 minutes"}
`
	path := writeFile(t, dir, "transcript.jsonl", content)
	// Invalid lines are skipped, valid lines still processed
	results, err := ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (skipping invalid JSON), got %d", len(results))
	}
}

func TestScanFile_BlankLines(t *testing.T) {
	dir := t.TempDir()
	content := `
{"role":"assistant","content":"I'll check back in 5 minutes"}

`
	path := writeFile(t, dir, "transcript.jsonl", content)
	results, err := ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (skipping blanks), got %d", len(results))
	}
}

func TestScanResult_HasLineNumber(t *testing.T) {
	dir := t.TempDir()
	content := `{"role":"user","content":"hello"}
{"role":"assistant","content":"I'll check back in 5 minutes"}
`
	path := writeFile(t, dir, "transcript.jsonl", content)
	results, err := ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].LineNumber != 2 {
		t.Errorf("expected line 2, got %d", results[0].LineNumber)
	}
}

func TestScanResult_HasDetectedAt(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "transcript.jsonl", `{"role":"assistant","content":"I'll check back in 5 minutes"}
`)
	before := time.Now()
	results, err := ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	after := time.Now()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].DetectedAt.Before(before) || results[0].DetectedAt.After(after) {
		t.Errorf("DetectedAt %v not between %v and %v", results[0].DetectedAt, before, after)
	}
}

func TestFormatScanResults_MatchesPRDFormat(t *testing.T) {
	results := []ScanResult{
		{
			Text:       "I'll check back in 5 minutes",
			Category:   "temporal",
			DetectedAt: time.Date(2026, 2, 13, 14, 30, 22, 0, time.UTC),
			LineNumber: 3,
			Status:     "UNVERIFIED",
		},
	}
	output := FormatScanResults(results)

	if !strings.Contains(output, "[COMMITMENT DETECTED]") {
		t.Error("missing [COMMITMENT DETECTED] header")
	}
	if !strings.Contains(output, "Text: I'll check back in 5 minutes") {
		t.Error("missing Text line")
	}
	if !strings.Contains(output, "Category: temporal") {
		t.Error("missing Category line")
	}
	if !strings.Contains(output, "Detected at: 2026-02-13 14:30:22") {
		t.Error("missing Detected at line")
	}
	if !strings.Contains(output, "Status: UNVERIFIED") {
		t.Error("missing Status line")
	}
}

func TestFormatScanResults_MultipleResults(t *testing.T) {
	results := []ScanResult{
		{
			Text:       "I'll check back in 5 minutes",
			Category:   "temporal",
			DetectedAt: time.Date(2026, 2, 13, 14, 30, 22, 0, time.UTC),
			LineNumber: 1,
			Status:     "UNVERIFIED",
		},
		{
			Text:       "Once the build finishes, I'll notify you",
			Category:   "conditional",
			DetectedAt: time.Date(2026, 2, 13, 14, 32, 15, 0, time.UTC),
			LineNumber: 5,
			Status:     "UNVERIFIED",
		},
	}
	output := FormatScanResults(results)
	count := strings.Count(output, "[COMMITMENT DETECTED]")
	if count != 2 {
		t.Errorf("expected 2 [COMMITMENT DETECTED] headers, got %d", count)
	}
}

func TestFormatScanResults_NoResults(t *testing.T) {
	output := FormatScanResults(nil)
	if !strings.Contains(output, "No commitments detected") {
		t.Error("expected 'No commitments detected' message for empty results")
	}
}

func TestFormatScanResultsJSON(t *testing.T) {
	results := []ScanResult{
		{
			Text:       "I'll check back in 5 minutes",
			Category:   "temporal",
			DetectedAt: time.Date(2026, 2, 13, 14, 30, 22, 0, time.UTC),
			LineNumber: 1,
			Status:     "UNVERIFIED",
		},
	}
	output := FormatScanResultsJSON(results)
	if !strings.Contains(output, `"text"`) {
		t.Error("JSON output missing text field")
	}
	if !strings.Contains(output, `"category"`) {
		t.Error("JSON output missing category field")
	}
	if !strings.Contains(output, `"temporal"`) {
		t.Error("JSON output missing temporal value")
	}
}

func TestScanFile_MessageWithTimestamp(t *testing.T) {
	dir := t.TempDir()
	// Transcript may include timestamp field
	path := writeFile(t, dir, "transcript.jsonl", `{"role":"assistant","content":"I'll check back in 5 minutes","timestamp":"2026-02-13T14:30:22Z"}
`)
	results, err := ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// When timestamp is present, use it for DetectedAt
	expected := time.Date(2026, 2, 13, 14, 30, 22, 0, time.UTC)
	if !results[0].DetectedAt.Equal(expected) {
		t.Errorf("expected DetectedAt %v, got %v", expected, results[0].DetectedAt)
	}
}

func TestScanFile_NoCommitmentsInNormalChat(t *testing.T) {
	dir := t.TempDir()
	content := `{"role":"assistant","content":"Sure, I can help with that."}
{"role":"assistant","content":"The function looks correct to me."}
{"role":"assistant","content":"You should run the tests to verify."}
`
	path := writeFile(t, dir, "transcript.jsonl", content)
	results, err := ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for normal chat, got %d", len(results))
	}
}
