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
	CategoryUntracked      Category = "untracked_problem"
	CategorySpeculative    Category = "speculative"
	CategoryWeakCommitment Category = "weak_commitment"
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
	patterns        []*regexp.Regexp
	conditionals    []*regexp.Regexp
	followups       []*regexp.Regexp
	weakCommitments []*regexp.Regexp
	codeUntracked   []*regexp.Regexp
	exclusions      []*regexp.Regexp
	pastTense       []*regexp.Regexp
	untracked       []*regexp.Regexp
	speculative     []*regexp.Regexp
	evidenceMarkers []*regexp.Regexp
	trackingMarkers []*regexp.Regexp
}

var (
	commitmentPatterns = []*regexp.Regexp{
		// Time-based commitments from first-person language.
		regexp.MustCompile(`(?i)\b(I'll|I will|I'm going to|I am going to|let me)\s+\w+.*\bin\s+\d+\s*(s|sec|secs|second|seconds|m|min|mins|minute|minutes|h|hr|hrs|hour|hours)\b`),
		// "I'll/I will check back" variations.
		regexp.MustCompile(`(?i)\b(I'll|I will|I'm going to|I am going to)\s+check\s+(back|in|again)\b`),
	}

	conditionalPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(once|when|after|as soon as|if)\b.+,?\s*(I'll|I will)\s+\w+`),
	}

	exclusionPatterns = []*regexp.Regexp{
		// Third-person system subjects: "the X will", "X.sh will".
		regexp.MustCompile(`(?i)^(the\s+\w+(\s+\w+)?|[\w.-]+\.(sh|py|js|go|rb|pl))\s+will\b`),
		// Pronoun subjects that aren't the agent: "it will", "this will", "that will".
		regexp.MustCompile(`(?i)^(it|this|that)\s+will\b`),
		// "that X will" pattern (e.g., "that process will continue").
		regexp.MustCompile(`(?i)^that\s+\w+\s+will\b`),
		// User instructions: "you can/should/will".
		regexp.MustCompile(`(?i)^you\s+(can|should|will|could|might)\b`),
	}

	pastTensePatterns = []*regexp.Regexp{
		// "I created/set up/configured/checked/ran/added/..." (simple past).
		regexp.MustCompile(`(?i)^I\s+(created|configured|checked|ran|added|scheduled|started|deployed|monitored|resolved|set\s+up)\b`),
		// "I already ..." (explicit past marker).
		regexp.MustCompile(`(?i)^I\s+already\b`),
		// "I've already/I've configured/I've set" (present perfect with contraction).
		regexp.MustCompile(`(?i)^I've\s+`),
		// "I have created/I have set up" (present perfect without contraction).
		regexp.MustCompile(`(?i)^I\s+have\s+(created|configured|checked|added|scheduled|started|deployed|monitored|resolved|set\s+up)\b`),
	}

	untrackedPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bthat'?s\s+a\s+separate\s+(fix|issue|problem|bug)\b`),
		regexp.MustCompile(`(?i)\bthere'?s\s+(a|an)\s+(failure|error|issue|bug|problem)\s+but\b`),
		regexp.MustCompile(`(?i)\bknown\s+(issue|bug|problem)\b`),
		regexp.MustCompile(`(?i)\bnot\s+related\s+to\s+this\s+(task|work|bead)\b`),
		regexp.MustCompile(`(?i)\bwill\s+need\s+to\s+be\s+(fixed|addressed|looked\s+at)\s+(later|separately)\b`),
	}

	speculativePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\blikely\b`),
		regexp.MustCompile(`(?i)\bprobably\b`),
		regexp.MustCompile(`(?i)\bassuming\b`),
	}

	evidenceMarkerPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bbased\s+on\b`),
		regexp.MustCompile(`(?i)\baccording\s+to\b`),
		regexp.MustCompile(`(?i)\b(as\s+shown\s+in|shown\s+in)\s+the\s+logs\b`),
		regexp.MustCompile(`(?i)\bthe\s+logs\s+show\b`),
		regexp.MustCompile(`(?i)\bi\s+checked\b`),
		regexp.MustCompile(`(?i)\bi\s+investigated\b`),
	}

	trackingReferencePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bbd-\d+\b`),
		regexp.MustCompile(`(?i)\btracked\s+in\s+(bd-\d+|bead|issue|ticket)\b`),
		regexp.MustCompile(`(?i)\b(created|logged|filed)\s+(a\s+)?(bead|issue|ticket)\b`),
		regexp.MustCompile(`(?i)\bbead\s+#?\d+\b`),
	}

	followupPatterns = []*regexp.Regexp{
		// "I'll monitor/watch/keep an eye on"
		regexp.MustCompile(`(?i)\b(I'll|I will|I'm going to|I am going to)\s+(monitor|watch|keep\s+an\s+eye\s+on)\b`),
		// "I'll report back/update you/let you know"
		regexp.MustCompile(`(?i)\b(I'll|I will|I'm going to|I am going to)\s+(report\s+back|update\s+you|let\s+you\s+know)\b`),
	}

	weakCommitmentPatterns = []*regexp.Regexp{
		// "I need to X" / "I should X"
		regexp.MustCompile(`(?i)\bI\s+(need\s+to|should)\s+\w+`),
	}

	codeUntrackedPatterns = []*regexp.Regexp{
		// TODO/FIXME/HACK markers in code-like context
		regexp.MustCompile(`(?i)\b(TODO|FIXME|HACK)\b[:\s]`),
	}
)

// NewDetector creates a new commitment detector
func NewDetector() *Detector {
	return &Detector{
		patterns:        commitmentPatterns,
		conditionals:    conditionalPatterns,
		followups:       followupPatterns,
		weakCommitments: weakCommitmentPatterns,
		codeUntracked:   codeUntrackedPatterns,
		exclusions:      exclusionPatterns,
		pastTense:       pastTensePatterns,
		untracked:       untrackedPatterns,
		speculative:     speculativePatterns,
		evidenceMarkers: evidenceMarkerPatterns,
		trackingMarkers: trackingReferencePatterns,
	}
}

func (d *Detector) hasTrackingReference(message string) bool {
	for _, marker := range d.trackingMarkers {
		if marker.MatchString(message) {
			return true
		}
	}
	return false
}

func (d *Detector) hasEvidenceMarker(message string) bool {
	for _, marker := range d.evidenceMarkers {
		if marker.MatchString(message) {
			return true
		}
	}
	return false
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

	// Untracked problem matching: reported issue without explicit tracking reference.
	for _, pattern := range d.untracked {
		loc := pattern.FindStringIndex(message)
		if loc == nil {
			continue
		}

		if d.hasTrackingReference(message[loc[1]:]) {
			continue
		}

		commitmentText := extractCommitmentText(message)
		return DetectionResult{
			IsCommitment:   true,
			Category:       CategoryUntracked,
			CommitmentText: commitmentText,
			Confidence:     0.90,
		}
	}

	// Code untracked: TODO/FIXME/HACK markers without tracking reference.
	for _, pattern := range d.codeUntracked {
		if !pattern.MatchString(message) {
			continue
		}
		if d.hasTrackingReference(message) {
			continue
		}
		commitmentText := extractCommitmentText(message)
		return DetectionResult{
			IsCommitment:   true,
			Category:       CategoryUntracked,
			CommitmentText: commitmentText,
			Confidence:     0.90,
		}
	}

	// Speculative analysis matching: detect uncertainty language that lacks evidence.
	for _, pattern := range d.speculative {
		if !pattern.MatchString(message) {
			continue
		}

		if d.hasEvidenceMarker(message) {
			continue
		}

		commitmentText := extractCommitmentText(message)
		return DetectionResult{
			IsCommitment:   true,
			Category:       CategorySpeculative,
			CommitmentText: commitmentText,
			Confidence:     0.85,
		}
	}

	// Conditional commitment matching: "once X, I'll Y", "when X, I'll Y", etc.
	// Checked before followups because conditionals are more specific.
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

	// Followup commitment matching: "I'll monitor/watch/report back/let you know".
	for _, pattern := range d.followups {
		if pattern.MatchString(message) {
			commitmentText := extractCommitmentText(message)
			return DetectionResult{
				IsCommitment:   true,
				Category:       CategoryFollowup,
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

	// Weak commitment matching: "I need to X" / "I should X".
	for _, pattern := range d.weakCommitments {
		if pattern.MatchString(message) {
			commitmentText := extractCommitmentText(message)
			return DetectionResult{
				IsCommitment:   true,
				Category:       CategoryWeakCommitment,
				CommitmentText: commitmentText,
				Confidence:     0.70,
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
