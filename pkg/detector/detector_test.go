package detector

import (
	"testing"
)

// TestDetectTemporalCommitment tests detection of time-based commitments
// US-001: Detect "I'll check back in 5 minutes" style commitments
func TestDetectTemporalCommitment(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		expected bool
	}{
		{
			name:     "explicit time-based commitment",
			message:  "I'll check back in 5 minutes",
			expected: true,
		},
		{
			name:     "variant with different time",
			message:  "I will check back in 10 minutes",
			expected: true,
		},
		{
			name:     "commitment with hour",
			message:  "I'll check in 1 hour",
			expected: true,
		},
		{
			name:     "not a commitment - system description",
			message:  "the script will monitor this process",
			expected: false,
		},
		{
			name:     "not a commitment - past tense",
			message:  "I checked back already",
			expected: false,
		},
		{
			name:     "not a commitment - no time reference",
			message:  "I'll help you with that",
			expected: false,
		},
	}

	d := NewDetector()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.DetectCommitment(tt.message)
			if result.IsCommitment != tt.expected {
				t.Errorf("DetectCommitment(%q) = %v, want %v", tt.message, result.IsCommitment, tt.expected)
			}
			if result.IsCommitment && result.Category != CategoryTemporal {
				t.Errorf("Expected category %v for temporal commitment, got %v", CategoryTemporal, result.Category)
			}
		})
	}
}

// TestDetectCommitmentExtractsText verifies that commitment text is correctly extracted
func TestDetectCommitmentExtractsText(t *testing.T) {
	d := NewDetector()

	message := "I'll check back in 5 minutes to see if it's done"
	result := d.DetectCommitment(message)

	if !result.IsCommitment {
		t.Fatal("Expected commitment to be detected")
	}

	if result.CommitmentText == "" {
		t.Error("Expected commitment text to be extracted, got empty string")
	}
}
