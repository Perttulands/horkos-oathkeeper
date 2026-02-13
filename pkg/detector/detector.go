package detector

import (
	"regexp"
	"strings"
)

// Category represents the type of commitment
type Category string

const (
	CategoryTemporal    Category = "temporal"
	CategoryScheduled   Category = "scheduled"
	CategoryFollowup    Category = "followup"
	CategoryConditional Category = "conditional"
)

// DetectionResult contains the results of commitment detection
type DetectionResult struct {
	IsCommitment   bool
	Category       Category
	CommitmentText string
	Confidence     float64
}

// Detector identifies commitment language in messages
type Detector struct {
	patterns []*regexp.Regexp
}

// NewDetector creates a new commitment detector
func NewDetector() *Detector {
	// Pattern matching for temporal commitments
	// Matches: "I'll check back in 5 minutes", "I will check in 1 hour", etc.
	patterns := []*regexp.Regexp{
		// "I'll/I will" + action + "in" + time duration
		regexp.MustCompile(`(?i)\b(I'll|I will)\s+\w+.*\bin\s+\d+\s+(minute|minutes|hour|hours|second|seconds)\b`),
		// "I'll/I will check back" variations
		regexp.MustCompile(`(?i)\b(I'll|I will)\s+check\s+(back|in|again)`),
	}

	return &Detector{
		patterns: patterns,
	}
}

// DetectCommitment analyzes a message and returns detection results
func (d *Detector) DetectCommitment(message string) DetectionResult {
	// Pattern matching for high-confidence temporal commitments
	for _, pattern := range d.patterns {
		if pattern.MatchString(message) {
			// Extract the commitment text (use the full message for now, can refine later)
			commitmentText := extractCommitmentText(message)

			return DetectionResult{
				IsCommitment:   true,
				Category:       CategoryTemporal,
				CommitmentText: commitmentText,
				Confidence:     0.95, // High confidence for pattern matches
			}
		}
	}

	// No commitment detected
	return DetectionResult{
		IsCommitment: false,
		Confidence:   0.0,
	}
}

// extractCommitmentText extracts the relevant commitment phrase from the message
func extractCommitmentText(message string) string {
	// For now, return the full message
	// This can be refined to extract just the commitment phrase
	return strings.TrimSpace(message)
}
