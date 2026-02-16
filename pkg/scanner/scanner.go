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

// transcriptMessage represents a single line in a flat JSONL transcript
type transcriptMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp,omitempty"`
}

// openClawContentBlock represents one block in an OpenClaw message.content array
type openClawContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// openClawMessage represents the nested message object in OpenClaw transcripts
type openClawMessage struct {
	Role    string                 `json:"role"`
	Content []openClawContentBlock `json:"content"`
}

// openClawLine represents a single line in an OpenClaw JSONL transcript
type openClawLine struct {
	ID        string          `json:"id"`
	Message   openClawMessage `json:"message"`
	Timestamp string          `json:"timestamp,omitempty"`
}

// parseLine attempts to parse a JSONL line as either OpenClaw nested format or
// flat format. Returns the role, text content(s), and timestamp.
// OpenClaw format: {"id":"...","message":{"role":"...","content":[{"type":"text","text":"..."}]},"timestamp":"..."}
// Flat format:     {"role":"...","content":"...","timestamp":"..."}
func parseLine(data []byte) (role string, texts []string, timestamp string) {
	// Try OpenClaw nested format first
	var oc openClawLine
	if err := json.Unmarshal(data, &oc); err == nil && oc.Message.Role != "" {
		for _, block := range oc.Message.Content {
			if block.Type == "text" && block.Text != "" {
				texts = append(texts, block.Text)
			}
		}
		return oc.Message.Role, texts, oc.Timestamp
	}

	// Fall back to flat format
	var flat transcriptMessage
	if err := json.Unmarshal(data, &flat); err == nil && flat.Role != "" {
		if flat.Content != "" {
			texts = append(texts, flat.Content)
		}
		return flat.Role, texts, flat.Timestamp
	}

	return "", nil, ""
}

// ScanFile reads a JSONL transcript file and returns detected commitments.
// Each line is expected to be a JSON object with "role" and "content" fields.
// Only assistant messages are analyzed. Invalid/blank lines are skipped.
func ScanFile(path string) ([]ScanResult, error) {
	const initialBufferSize = 64 * 1024
	const maxBufferSize = 2 * 1024 * 1024

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open transcript: %w", err)
	}
	defer f.Close()

	d := detector.NewDetector()
	var results []ScanResult
	lineNum := 0

	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, initialBufferSize), maxBufferSize)
	for s.Scan() {
		lineNum++
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}

		role, texts, timestamp := parseLine([]byte(line))
		if role != "assistant" || len(texts) == 0 {
			continue
		}

		detectedAt := time.Now()
		if timestamp != "" {
			if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
				detectedAt = t
			}
		}

		for _, text := range texts {
			result := d.DetectCommitment(text)
			if !result.IsCommitment {
				continue
			}
			results = append(results, ScanResult{
				Text:       result.CommitmentText,
				Category:   string(result.Category),
				DetectedAt: detectedAt,
				LineNumber: lineNum,
				Status:     "UNVERIFIED",
			})
		}
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
