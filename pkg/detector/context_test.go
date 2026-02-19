package detector

import "testing"

func TestContextAnalyzerFulfillmentAcrossMessages(t *testing.T) {
	ca := NewContextAnalyzer(5)

	messages := []string{
		"I need to check the logs for errors",
		"Looking at the deployment now",
		"I checked the logs and found the issue",
	}

	result := ca.Analyze(messages)

	if len(result.Fulfilled) != 1 {
		t.Fatalf("expected 1 fulfilled commitment, got %d", len(result.Fulfilled))
	}
	if result.Fulfilled[0].FulfilledBy != "I checked the logs and found the issue" {
		t.Fatalf("unexpected fulfillment message: %q", result.Fulfilled[0].FulfilledBy)
	}
}

func TestContextAnalyzerFulfillmentWithDone(t *testing.T) {
	ca := NewContextAnalyzer(5)

	messages := []string{
		"I'll monitor the deployment for errors",
		"Still watching the metrics",
		"Done with the monitoring, all clear",
	}

	result := ca.Analyze(messages)

	if len(result.Fulfilled) != 1 {
		t.Fatalf("expected 1 fulfilled commitment, got %d", len(result.Fulfilled))
	}
}

func TestContextAnalyzerRepeatedPromisesEscalation(t *testing.T) {
	ca := NewContextAnalyzer(5)

	messages := []string{
		"I'll monitor the build output",
		"Let me look at something else first",
		"I'll watch the deployment logs",
	}

	result := ca.Analyze(messages)

	if len(result.Escalated) != 1 {
		t.Fatalf("expected 1 escalated category, got %d", len(result.Escalated))
	}
	if result.Escalated[0].Category != CategoryFollowup {
		t.Fatalf("expected followup category escalated, got %v", result.Escalated[0].Category)
	}
	if result.Escalated[0].Count != 2 {
		t.Fatalf("expected count=2, got %d", result.Escalated[0].Count)
	}
	if result.Escalated[0].Confidence != 0.95 {
		t.Fatalf("expected confidence 0.95 for 2 repeats, got %v", result.Escalated[0].Confidence)
	}
}

func TestContextAnalyzerNoFalsePositivesOnUnrelated(t *testing.T) {
	ca := NewContextAnalyzer(5)

	messages := []string{
		"Here's the status update",
		"The build completed successfully",
		"Moving on to the next task",
	}

	result := ca.Analyze(messages)

	if len(result.Fulfilled) != 0 {
		t.Fatalf("expected no fulfilled commitments, got %d", len(result.Fulfilled))
	}
	if len(result.Escalated) != 0 {
		t.Fatalf("expected no escalated commitments, got %d", len(result.Escalated))
	}
}

func TestContextAnalyzerWindowSizeDefault(t *testing.T) {
	ca := NewContextAnalyzer(0)

	if ca.windowSize != 5 {
		t.Fatalf("expected default window size 5, got %d", ca.windowSize)
	}
}

func TestContextAnalyzerWindowSizeLimitsMessages(t *testing.T) {
	ca := NewContextAnalyzer(2)

	// The commitment is in the first message, but with window=2 only last 2 messages are analyzed
	messages := []string{
		"I'll check the logs for errors",
		"Working on something else",
		"I checked the logs and everything is fine",
	}

	result := ca.Analyze(messages)

	// Window of 2 means only the last 2 messages are seen
	// The commitment in msg[0] is outside the window
	if len(result.Fulfilled) != 0 {
		t.Fatalf("expected no fulfilled commitments (commitment outside window), got %d", len(result.Fulfilled))
	}
}

func TestContextAnalyzerEmptyMessages(t *testing.T) {
	ca := NewContextAnalyzer(5)

	result := ca.Analyze([]string{})

	if len(result.Fulfilled) != 0 {
		t.Fatalf("expected no fulfilled commitments for empty input, got %d", len(result.Fulfilled))
	}
	if len(result.Escalated) != 0 {
		t.Fatalf("expected no escalated commitments for empty input, got %d", len(result.Escalated))
	}
}

func TestContextAnalyzerFulfilledNotEscalated(t *testing.T) {
	ca := NewContextAnalyzer(5)

	// Two commitments of the same type, but the first is fulfilled
	// Should not be escalated
	messages := []string{
		"I'll monitor the build",
		"I monitored the build and it passed",
		"I'll watch the deployment",
	}

	result := ca.Analyze(messages)

	if len(result.Fulfilled) != 1 {
		t.Fatalf("expected 1 fulfilled commitment, got %d", len(result.Fulfilled))
	}
	// Only 1 unfulfilled followup commitment remains, not enough for escalation
	if len(result.Escalated) != 0 {
		t.Fatalf("expected no escalation (only 1 unfulfilled), got %d", len(result.Escalated))
	}
}

func TestContextAnalyzerTripleEscalation(t *testing.T) {
	ca := NewContextAnalyzer(5)

	messages := []string{
		"I need to fix the tests",
		"I should update the docs",
		"I need to check the coverage",
	}

	result := ca.Analyze(messages)

	if len(result.Escalated) != 1 {
		t.Fatalf("expected 1 escalated category, got %d", len(result.Escalated))
	}
	if result.Escalated[0].Count != 3 {
		t.Fatalf("expected count=3, got %d", result.Escalated[0].Count)
	}
	if result.Escalated[0].Confidence != 1.0 {
		t.Fatalf("expected confidence 1.0 for 3+ repeats, got %v", result.Escalated[0].Confidence)
	}
}

func TestContextAnalyzerMinConfidenceAffectsEscalation(t *testing.T) {
	ca := NewContextAnalyzerWithMinConfidence(5, 0.8)

	messages := []string{
		"I need to fix the tests",
		"I should update the docs",
	}

	result := ca.Analyze(messages)
	if len(result.Escalated) != 0 {
		t.Fatalf("expected no escalation when weak commitments are below threshold, got %d", len(result.Escalated))
	}
}
