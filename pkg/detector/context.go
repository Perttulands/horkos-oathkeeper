package detector

import (
	"strings"
)

// ContextAnalyzer analyzes sequences of messages to detect fulfilled
// commitments and repeated promises across a conversation window.
type ContextAnalyzer struct {
	windowSize int
	detector   *Detector
}

// FulfilledCommitment represents a commitment that was detected as fulfilled
// based on subsequent messages in the conversation.
type FulfilledCommitment struct {
	CommitmentText string
	FulfilledBy    string
}

// EscalatedCommitment represents a commitment type that was repeated,
// indicating the agent may be making promises without following through.
type EscalatedCommitment struct {
	Category   Category
	Count      int
	Confidence float64
}

// ContextResult holds the results of context-aware analysis across messages.
type ContextResult struct {
	Fulfilled []FulfilledCommitment
	Escalated []EscalatedCommitment
}

// NewContextAnalyzer creates a ContextAnalyzer with the given window size.
// If windowSize <= 0, defaults to 5.
func NewContextAnalyzer(windowSize int) *ContextAnalyzer {
	return NewContextAnalyzerWithMinConfidence(windowSize, DefaultMinConfidence)
}

// NewContextAnalyzerWithMinConfidence creates a ContextAnalyzer with a
// configurable detector confidence threshold.
func NewContextAnalyzerWithMinConfidence(windowSize int, minConfidence float64) *ContextAnalyzer {
	if windowSize <= 0 {
		windowSize = 5
	}
	return &ContextAnalyzer{
		windowSize: windowSize,
		detector:   NewDetectorWithMinConfidence(minConfidence),
	}
}

// Analyze examines the last N messages and returns fulfilled commitments
// and escalated (repeated) commitment types.
func (ca *ContextAnalyzer) Analyze(messages []string) ContextResult {
	result := ContextResult{
		Fulfilled: []FulfilledCommitment{},
		Escalated: []EscalatedCommitment{},
	}

	if len(messages) == 0 {
		return result
	}

	// Apply window size limit
	window := messages
	if len(window) > ca.windowSize {
		window = window[len(window)-ca.windowSize:]
	}

	// Pass 1: Detect commitments in each message
	type commitmentEntry struct {
		index  int
		result DetectionResult
		text   string
	}
	var commitments []commitmentEntry
	for i, msg := range window {
		det := ca.detector.DetectCommitment(msg)
		if det.IsCommitment {
			commitments = append(commitments, commitmentEntry{
				index:  i,
				result: det,
				text:   strings.TrimSpace(msg),
			})
		}
	}

	// Pass 2: Check for fulfillment — a later message resolves an earlier commitment
	fulfilled := map[int]bool{}
	for _, c := range commitments {
		verb := extractCommitmentVerb(c.text)
		if verb == "" {
			continue
		}
		// Look at messages after this commitment for fulfillment
		for j := c.index + 1; j < len(window); j++ {
			if isFulfillment(verb, window[j]) {
				result.Fulfilled = append(result.Fulfilled, FulfilledCommitment{
					CommitmentText: c.text,
					FulfilledBy:    strings.TrimSpace(window[j]),
				})
				fulfilled[c.index] = true
				break
			}
		}
	}

	// Pass 3: Detect repeated commitment types (escalation)
	categoryCounts := map[Category]int{}
	for _, c := range commitments {
		if fulfilled[c.index] {
			continue
		}
		categoryCounts[c.result.Category]++
	}

	for cat, count := range categoryCounts {
		if count >= 2 {
			result.Escalated = append(result.Escalated, EscalatedCommitment{
				Category:   cat,
				Count:      count,
				Confidence: escalatedConfidence(count),
			})
		}
	}

	return result
}

// extractCommitmentVerb pulls the action verb from commitment phrases like
// "I'll check X" → "check", "I need to fix Y" → "fix".
func extractCommitmentVerb(text string) string {
	lower := strings.ToLower(text)

	// Try "I'll/I will/I'm going to <verb>" patterns
	prefixes := []string{
		"i'll ", "i will ", "i'm going to ", "i am going to ",
		"let me ", "i need to ", "i should ",
	}
	for _, prefix := range prefixes {
		idx := strings.Index(lower, prefix)
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(lower[idx+len(prefix):])
		words := strings.Fields(rest)
		if len(words) > 0 {
			return words[0]
		}
	}
	return ""
}

// isFulfillment checks if a message indicates that the given verb action
// was completed. E.g., verb="check" matches "I checked the logs".
func isFulfillment(verb string, message string) bool {
	lower := strings.ToLower(message)
	verb = strings.ToLower(verb)

	// Map present tense verbs to their past tense forms
	pastForms := pastTenseForms(verb)

	for _, past := range pastForms {
		if strings.Contains(lower, "i "+past) || strings.Contains(lower, "i've "+past) || strings.Contains(lower, "i have "+past) {
			return true
		}
	}

	// Also check "done", "completed", "finished" as generic fulfillment
	for _, indicator := range []string{"done", "completed", "finished"} {
		if strings.Contains(lower, indicator) {
			return true
		}
	}

	return false
}

// pastTenseForms returns likely past-tense forms for a verb.
func pastTenseForms(verb string) []string {
	// Common irregular verbs
	irregulars := map[string][]string{
		"check":   {"checked"},
		"run":     {"ran"},
		"fix":     {"fixed"},
		"monitor": {"monitored"},
		"watch":   {"watched"},
		"report":  {"reported"},
		"update":  {"updated"},
		"deploy":  {"deployed"},
		"verify":  {"verified"},
		"look":    {"looked"},
		"set":     {"set"},
		"get":     {"got", "gotten"},
		"make":    {"made"},
		"write":   {"wrote", "written"},
		"send":    {"sent"},
		"build":   {"built"},
		"find":    {"found"},
		"test":    {"tested"},
		"add":     {"added"},
		"create":  {"created"},
	}
	if forms, ok := irregulars[verb]; ok {
		return forms
	}

	// Regular verb: add "ed" suffix
	if strings.HasSuffix(verb, "e") {
		return []string{verb + "d"}
	}
	return []string{verb + "ed"}
}

// escalatedConfidence returns the confidence for repeated commitments.
func escalatedConfidence(count int) float64 {
	if count >= 3 {
		return 1.0
	}
	return 0.95
}
