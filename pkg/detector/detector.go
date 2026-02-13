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
	patterns     []*regexp.Regexp
	conditionals []*regexp.Regexp
	exclusions   []*regexp.Regexp
	pastTense    []*regexp.Regexp
}

// NewDetector creates a new commitment detector
func NewDetector() *Detector {
	// Commitment patterns: match first-person agent promises with time references
	patterns := []*regexp.Regexp{
		// "I'll/I will" + action + "in" + time duration
		regexp.MustCompile(`(?i)\b(I'll|I will)\s+\w+.*\bin\s+\d+\s+(minute|minutes|hour|hours|second|seconds)\b`),
		// "I'll/I will check back" variations
		regexp.MustCompile(`(?i)\b(I'll|I will)\s+check\s+(back|in|again)`),
	}

	// Conditional commitment patterns: "once/when/after/if X, I'll/I will Y"
	conditionals := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(once|when|after|as soon as|if)\b.+,?\s*(I'll|I will)\s+\w+`),
	}

	// Exclusion patterns: non-agent subjects that describe system behavior or user instructions
	exclusions := []*regexp.Regexp{
		// Third-person system subjects: "the X will", "X.sh will"
		regexp.MustCompile(`(?i)^(the\s+\w+(\s+\w+)?|[\w.-]+\.(sh|py|js|go|rb|pl))\s+will\b`),
		// Pronoun subjects that aren't the agent: "it will", "this will", "that will"
		regexp.MustCompile(`(?i)^(it|this|that)\s+will\b`),
		// "that X will" pattern (e.g., "that process will continue")
		regexp.MustCompile(`(?i)^that\s+\w+\s+will\b`),
		// User instructions: "you can/should/will"
		regexp.MustCompile(`(?i)^you\s+(can|should|will|could|might)\b`),
	}

	// Past-tense patterns: actions already completed (not commitments)
	pastTense := []*regexp.Regexp{
		// "I created/set up/configured/checked/ran/added/..." (simple past)
		regexp.MustCompile(`(?i)^I\s+(created|configured|checked|ran|added|scheduled|started|deployed|monitored|resolved|set\s+up)\b`),
		// "I already ..." (explicit past marker)
		regexp.MustCompile(`(?i)^I\s+already\b`),
		// "I've already/I've configured/I've set" (present perfect with contraction)
		regexp.MustCompile(`(?i)^I've\s+`),
		// "I have created/I have set up" (present perfect without contraction)
		regexp.MustCompile(`(?i)^I\s+have\s+(created|configured|checked|added|scheduled|started|deployed|monitored|resolved|set\s+up)\b`),
	}

	return &Detector{
		patterns:     patterns,
		conditionals: conditionals,
		exclusions:   exclusions,
		pastTense:    pastTense,
	}
}

// IsSystemDescription returns true if the message describes system behavior
// or user instructions rather than an agent commitment.
func (d *Detector) IsSystemDescription(message string) bool {
	trimmed := strings.TrimSpace(message)
	for _, pattern := range d.exclusions {
		if pattern.MatchString(trimmed) {
			return true
		}
	}
	return false
}

// IsPastTenseAction returns true if the message describes a completed action
// rather than a future commitment. For example, "I created a cron job" is past
// tense and should not be treated as a commitment.
func (d *Detector) IsPastTenseAction(message string) bool {
	trimmed := strings.TrimSpace(message)
	for _, pattern := range d.pastTense {
		if pattern.MatchString(trimmed) {
			return true
		}
	}
	return false
}

// DetectCommitment analyzes a message and returns detection results
func (d *Detector) DetectCommitment(message string) DetectionResult {
	// Exclude system descriptions and user instructions first
	if d.IsSystemDescription(message) {
		return DetectionResult{
			IsCommitment: false,
			Confidence:   0.0,
		}
	}

	// Exclude past-tense actions (completed work, not commitments)
	if d.IsPastTenseAction(message) {
		return DetectionResult{
			IsCommitment: false,
			Confidence:   0.0,
		}
	}

	// Conditional commitment matching: "once X, I'll Y", "when X, I'll Y", etc.
	for _, pattern := range d.conditionals {
		if pattern.MatchString(message) {
			commitmentText := extractCommitmentText(message)

			return DetectionResult{
				IsCommitment:   true,
				Category:       CategoryConditional,
				CommitmentText: commitmentText,
				Confidence:     0.90,
			}
		}
	}

	// Pattern matching for high-confidence temporal commitments
	for _, pattern := range d.patterns {
		if pattern.MatchString(message) {
			commitmentText := extractCommitmentText(message)

			return DetectionResult{
				IsCommitment:   true,
				Category:       CategoryTemporal,
				CommitmentText: commitmentText,
				Confidence:     0.95,
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
