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

// TestExcludeSystemBehaviorDescriptions verifies that system descriptions are NOT detected as commitments
// US-002: Ignore descriptions like "the script will monitor this process"
func TestExcludeSystemBehaviorDescriptions(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{
			name:    "script will monitor",
			message: "the script will monitor this process",
		},
		{
			name:    "cron job will run",
			message: "the cron job will run every 5 minutes",
		},
		{
			name:    "service will restart",
			message: "the service will restart automatically",
		},
		{
			name:    "daemon will handle",
			message: "the daemon will handle reconnections",
		},
		{
			name:    "watcher will detect",
			message: "the watcher will detect file changes",
		},
		{
			name:    "tool will process",
			message: "dispatch.sh will process the queue",
		},
		{
			name:    "agent will execute",
			message: "the build agent will execute the pipeline",
		},
		{
			name:    "system description with it",
			message: "it will automatically retry on failure",
		},
		{
			name:    "system description with this",
			message: "this will run in the background",
		},
		{
			name:    "system description with that",
			message: "that process will continue monitoring",
		},
		{
			name:    "the command will",
			message: "the command will check the status every minute",
		},
		{
			name:    "user instruction - you can",
			message: "you can check the logs in 5 minutes",
		},
		{
			name:    "user instruction - you should",
			message: "you should monitor the process",
		},
	}

	d := NewDetector()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.DetectCommitment(tt.message)
			if result.IsCommitment {
				t.Errorf("DetectCommitment(%q) = true, want false (should be excluded as system description)", tt.message)
			}
		})
	}
}

// TestIsSystemDescription verifies direct classification of system vs agent language
// US-002: Core exclusion logic
func TestIsSystemDescription(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		expected bool
	}{
		// System descriptions (should be excluded)
		{name: "the script will", message: "the script will monitor this process", expected: true},
		{name: "the cron job will", message: "the cron job will run every 5 minutes", expected: true},
		{name: "the service will", message: "the service will restart automatically", expected: true},
		{name: "the daemon will", message: "the daemon will handle reconnections", expected: true},
		{name: "the watcher will", message: "the watcher will detect file changes", expected: true},
		{name: "tool.sh will", message: "dispatch.sh will process the queue", expected: true},
		{name: "the build agent will", message: "the build agent will execute the pipeline", expected: true},
		{name: "it will", message: "it will automatically retry on failure", expected: true},
		{name: "this will", message: "this will run in the background", expected: true},
		{name: "that will", message: "that process will continue monitoring", expected: true},
		{name: "the command will", message: "the command will check the status every minute", expected: true},
		// User instructions (should be excluded)
		{name: "you can", message: "you can check the logs in 5 minutes", expected: true},
		{name: "you should", message: "you should monitor the process", expected: true},
		{name: "you will", message: "you will need to restart the service", expected: true},
		// Agent language (should NOT be excluded)
		{name: "I'll", message: "I'll check back in 5 minutes", expected: false},
		{name: "I will", message: "I will monitor the build", expected: false},
		{name: "let me", message: "let me check on that for you", expected: false},
		{name: "I'm going to", message: "I'm going to run the tests", expected: false},
	}

	d := NewDetector()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.IsSystemDescription(tt.message)
			if result != tt.expected {
				t.Errorf("IsSystemDescription(%q) = %v, want %v", tt.message, result, tt.expected)
			}
		})
	}
}

// TestAgentCommitmentsStillDetected ensures real agent commitments are not excluded by system description filtering
// US-002: Agent commitments with "I" subject should still be detected
func TestAgentCommitmentsStillDetected(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{
			name:    "I'll check back in 5 minutes",
			message: "I'll check back in 5 minutes",
		},
		{
			name:    "I will monitor and check in 10 minutes",
			message: "I will monitor this and check back in 10 minutes",
		},
	}

	d := NewDetector()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.DetectCommitment(tt.message)
			if !result.IsCommitment {
				t.Errorf("DetectCommitment(%q) = false, want true (agent commitment should be detected)", tt.message)
			}
		})
	}
}

// TestExcludePastTenseActions verifies that past-tense actions are NOT detected as commitments
// US-003: Distinguish "I created a cron job" (past) from "I'll create a cron job" (future)
func TestExcludePastTenseActions(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{name: "I created", message: "I created a cron job to handle that"},
		{name: "I set up", message: "I set up a monitoring script"},
		{name: "I configured", message: "I configured the watcher to check every 5 minutes"},
		{name: "I already checked", message: "I already checked back on it"},
		{name: "I ran", message: "I ran the tests and they passed"},
		{name: "I added", message: "I added a bead to track that process"},
		{name: "I scheduled", message: "I scheduled a cron job for 3pm"},
		{name: "I started", message: "I started a tmux session to monitor it"},
		{name: "I deployed", message: "I deployed the fix 10 minutes ago"},
		{name: "I've already", message: "I've already set that up"},
		{name: "I have created", message: "I have created the backing mechanism"},
		{name: "I've configured", message: "I've configured automatic retries"},
	}

	d := NewDetector()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.DetectCommitment(tt.message)
			if result.IsCommitment {
				t.Errorf("DetectCommitment(%q) = true, want false (past-tense action should not be a commitment)", tt.message)
			}
		})
	}
}

// TestIsPastTenseAction verifies direct classification of past-tense actions
// US-003: Core past-tense detection logic
func TestIsPastTenseAction(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		expected bool
	}{
		// Past-tense actions (should be recognized)
		{name: "I created", message: "I created a cron job to handle that", expected: true},
		{name: "I set up", message: "I set up a monitoring script", expected: true},
		{name: "I configured", message: "I configured the watcher to check every 5 minutes", expected: true},
		{name: "I checked", message: "I already checked back on it", expected: true},
		{name: "I ran", message: "I ran the tests", expected: true},
		{name: "I added", message: "I added a bead to track that", expected: true},
		{name: "I scheduled", message: "I scheduled a cron job for 3pm", expected: true},
		{name: "I started", message: "I started a tmux session", expected: true},
		{name: "I deployed", message: "I deployed the fix", expected: true},
		{name: "I've already", message: "I've already set that up", expected: true},
		{name: "I have created", message: "I have created the mechanism", expected: true},
		{name: "I've configured", message: "I've configured retries", expected: true},
		{name: "I monitored", message: "I monitored the build process", expected: true},
		{name: "I resolved", message: "I resolved the issue earlier", expected: true},
		// Future commitments (should NOT be classified as past-tense)
		{name: "I'll create", message: "I'll create a cron job", expected: false},
		{name: "I will check", message: "I will check back in 5 minutes", expected: false},
		{name: "I'm going to", message: "I'm going to monitor the build", expected: false},
		{name: "let me", message: "let me set that up for you", expected: false},
	}

	d := NewDetector()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.IsPastTenseAction(tt.message)
			if result != tt.expected {
				t.Errorf("IsPastTenseAction(%q) = %v, want %v", tt.message, result, tt.expected)
			}
		})
	}
}

// TestFutureCommitmentsStillDetectedAfterPastTenseFilter ensures future commitments are not blocked by past-tense filter
// US-003: "I'll create a cron job" should still be detected as a commitment
func TestFutureCommitmentsStillDetectedAfterPastTenseFilter(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{name: "I'll check back", message: "I'll check back in 5 minutes"},
		{name: "I will monitor", message: "I will monitor this and check back in 10 minutes"},
		{name: "I'll check in", message: "I'll check in 1 hour"},
	}

	d := NewDetector()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.DetectCommitment(tt.message)
			if !result.IsCommitment {
				t.Errorf("DetectCommitment(%q) = false, want true (future commitment should be detected)", tt.message)
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
