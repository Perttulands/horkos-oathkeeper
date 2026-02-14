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

func TestDetectTemporalCommitmentVariants(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{
			name:    "im going to with duration",
			message: "I'm going to check back in 5 minutes",
		},
		{
			name:    "i am going to with hour",
			message: "I am going to verify this and check in 1 hour",
		},
		{
			name:    "let me with duration",
			message: "let me check that and report back in 30 seconds",
		},
	}

	d := NewDetector()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.DetectCommitment(tt.message)
			if !result.IsCommitment {
				t.Errorf("DetectCommitment(%q) = false, want true", tt.message)
			}
		})
	}
}

func TestNewDetectorReusesCompiledRegexes(t *testing.T) {
	d1 := NewDetector()
	d2 := NewDetector()

	if d1.patterns[0] != d2.patterns[0] {
		t.Fatal("expected commitment patterns to be compiled once and reused")
	}
	if d1.conditionals[0] != d2.conditionals[0] {
		t.Fatal("expected conditional patterns to be compiled once and reused")
	}
	if d1.untracked[0] != d2.untracked[0] {
		t.Fatal("expected untracked patterns to be compiled once and reused")
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

// TestDetectConditionalCommitments verifies that conditional commitments are detected
// US-004: Detect "once the build finishes, I'll notify you" style commitments
func TestDetectConditionalCommitments(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{name: "once X I'll Y", message: "once the build finishes, I'll notify you"},
		{name: "once X I will Y", message: "once the tests pass, I will deploy the fix"},
		{name: "when X I'll Y", message: "when the migration completes, I'll run the verification"},
		{name: "when X I will Y", message: "when done, I will report back"},
		{name: "after X I'll Y", message: "after the agent finishes, I'll check the results"},
		{name: "after X I will Y", message: "after deployment completes, I will verify the endpoints"},
		{name: "as soon as X I'll Y", message: "as soon as the build is ready, I'll let you know"},
		{name: "if X I'll Y", message: "if the tests pass, I'll merge the PR"},
		{name: "if X I will Y", message: "if that succeeds, I will proceed with the rollout"},
	}

	d := NewDetector()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.DetectCommitment(tt.message)
			if !result.IsCommitment {
				t.Errorf("DetectCommitment(%q) = false, want true (conditional commitment should be detected)", tt.message)
			}
			if result.Category != CategoryConditional {
				t.Errorf("DetectCommitment(%q) category = %v, want %v", tt.message, result.Category, CategoryConditional)
			}
		})
	}
}

// TestConditionalCommitmentsNotConfusedWithExclusions verifies conditional patterns don't conflict with exclusion filters
// US-004: "once the build finishes, I'll notify you" should NOT be excluded by system description filter
func TestConditionalCommitmentsNotConfusedWithExclusions(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{name: "once + system subject", message: "once the build finishes, I'll notify you"},
		{name: "when + system subject", message: "when the script completes, I'll report back"},
		{name: "after + system subject", message: "after the agent finishes, I'll check the results"},
	}

	d := NewDetector()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// These should NOT be classified as system descriptions
			if d.IsSystemDescription(tt.message) {
				t.Errorf("IsSystemDescription(%q) = true, want false (conditional commitment, not system description)", tt.message)
			}
			// These should be detected as commitments
			result := d.DetectCommitment(tt.message)
			if !result.IsCommitment {
				t.Errorf("DetectCommitment(%q) = false, want true", tt.message)
			}
		})
	}
}

// TestNonCommitmentConditionalsExcluded verifies that conditional system descriptions are NOT detected
// US-004: "if the script fails, it will retry" is not an agent commitment
func TestNonCommitmentConditionalsExcluded(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{name: "conditional system behavior", message: "if the script fails, it will retry automatically"},
		{name: "conditional system with that", message: "once the build finishes, that will trigger the deploy"},
		{name: "conditional with you", message: "when the build is done, you can check the results"},
	}

	d := NewDetector()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.DetectCommitment(tt.message)
			if result.IsCommitment {
				t.Errorf("DetectCommitment(%q) = true, want false (not an agent commitment)", tt.message)
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

// TestDetectUntrackedProblem verifies detection of untracked problem statements
func TestDetectUntrackedProblem(t *testing.T) {
	tests := []struct {
		name           string
		message        string
		expectDetected bool
		expectCategory Category
	}{
		{
			name:           "separate fix without tracking",
			message:        "that's a separate fix",
			expectDetected: true,
			expectCategory: CategoryUntracked,
		},
		{
			name:           "failure but exits cleanly",
			message:        "there's a failure but it exits cleanly",
			expectDetected: true,
			expectCategory: CategoryUntracked,
		},
		{
			name:           "known issue without tracking",
			message:        "known issue",
			expectDetected: true,
			expectCategory: CategoryUntracked,
		},
		{
			name:           "not related to this task",
			message:        "but that's not related to this task",
			expectDetected: true,
			expectCategory: CategoryUntracked,
		},
		{
			name:           "separate fix with bead tracking",
			message:        "that's a separate fix. Created bead bd-123",
			expectDetected: false,
		},
		{
			name:           "known issue tracked in bead",
			message:        "known issue, tracked in bd-456",
			expectDetected: false,
		},
		{
			name:           "past tense separate issue fixed",
			message:        "I fixed the separate issue",
			expectDetected: false,
		},
	}

	d := NewDetector()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.DetectCommitment(tt.message)

			if result.IsCommitment != tt.expectDetected {
				t.Errorf("DetectCommitment(%q) detected = %v, want %v", tt.message, result.IsCommitment, tt.expectDetected)
			}

			if tt.expectDetected && result.Category != tt.expectCategory {
				t.Errorf("DetectCommitment(%q) category = %v, want %v", tt.message, result.Category, tt.expectCategory)
			}
		})
	}
}

func TestDetectUntrackedProblemNotTrackedYet(t *testing.T) {
	d := NewDetector()

	message := "that's a separate fix, not tracked yet"
	result := d.DetectCommitment(message)

	if !result.IsCommitment {
		t.Fatalf("DetectCommitment(%q) = false, want true", message)
	}
	if result.Category != CategoryUntracked {
		t.Fatalf("DetectCommitment(%q) category = %v, want %v", message, result.Category, CategoryUntracked)
	}
}

func TestDetectSpeculativeLanguageWithoutEvidence(t *testing.T) {
	tests := []struct {
		name           string
		message        string
		expectDetected bool
		expectCategory Category
	}{
		{
			name:           "likely without evidence",
			message:        "Likely hit issues and bailed",
			expectDetected: true,
			expectCategory: CategorySpeculative,
		},
		{
			name:           "probably without evidence",
			message:        "Probably a test failure",
			expectDetected: true,
			expectCategory: CategorySpeculative,
		},
		{
			name:           "assuming without evidence",
			message:        "I'm assuming the agent failed",
			expectDetected: true,
			expectCategory: CategorySpeculative,
		},
		{
			name:           "likely with based on evidence",
			message:        "This is likely caused by X based on the error output",
			expectDetected: false,
		},
		{
			name:           "probably with logs evidence",
			message:        "The test probably fails because of Y, as shown in the logs",
			expectDetected: false,
		},
		{
			name:           "likely after investigation",
			message:        "I investigated and it was likely caused by X",
			expectDetected: false,
		},
	}

	d := NewDetector()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.DetectCommitment(tt.message)

			if result.IsCommitment != tt.expectDetected {
				t.Errorf("DetectCommitment(%q) detected = %v, want %v", tt.message, result.IsCommitment, tt.expectDetected)
			}

			if tt.expectDetected && result.Category != tt.expectCategory {
				t.Errorf("DetectCommitment(%q) category = %v, want %v", tt.message, result.Category, tt.expectCategory)
			}
		})
	}
}
