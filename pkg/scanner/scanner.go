package scanner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/perttulands/oathkeeper/pkg/detector"
)

// ScanResult represents a single detected commitment from a transcript scan
type ScanResult struct {
	Text       string    `json:"text"`
	Category   string    `json:"category"`
	DetectedAt time.Time `json:"detected_at"`
	LineNumber int       `json:"line_number"`
	Status     string    `json:"status"`
}

// transcriptMessage represents a single line in a JSONL transcript
type transcriptMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp,omitempty"`
}

// ScanFile reads a JSONL transcript file and returns detected commitments.
// Each line is expected to be a JSON object with "role" and "content" fields.
// Only assistant messages are analyzed. Invalid/blank lines are skipped.
func ScanFile(path string) ([]ScanResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open transcript: %w", err)
	}
	defer f.Close()

	d := detector.NewDetector()
	var results []ScanResult
	lineNum := 0

	s := bufio.NewScanner(f)
	for s.Scan() {
		lineNum++
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}

		var msg transcriptMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue // skip invalid JSON lines
		}

		if msg.Role != "assistant" {
			continue
		}

		result := d.DetectCommitment(msg.Content)
		if !result.IsCommitment {
			continue
		}

		detectedAt := time.Now()
		if msg.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339, msg.Timestamp); err == nil {
				detectedAt = t
			}
		}

		results = append(results, ScanResult{
			Text:       result.CommitmentText,
			Category:   string(result.Category),
			DetectedAt: detectedAt,
			LineNumber: lineNum,
			Status:     "UNVERIFIED",
		})
	}

	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("read transcript: %w", err)
	}

	return results, nil
}

// FormatScanResults formats scan results as human-readable text matching
// the PRD section 7 output format for `oathkeeper scan`.
func FormatScanResults(results []ScanResult) string {
	if len(results) == 0 {
		return "No commitments detected.\n"
	}

	var b strings.Builder
	for i, r := range results {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "[COMMITMENT DETECTED]\n")
		fmt.Fprintf(&b, "Text: %s\n", r.Text)
		fmt.Fprintf(&b, "Category: %s\n", r.Category)
		fmt.Fprintf(&b, "Detected at: %s\n", r.DetectedAt.Format("2006-01-02 15:04:05"))
		fmt.Fprintf(&b, "Mechanisms found: (none)\n")
		fmt.Fprintf(&b, "Status: %s\n", r.Status)
	}
	return b.String()
}

// FormatScanResultsJSON formats scan results as JSON.
func FormatScanResultsJSON(results []ScanResult) string {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(data)
}
