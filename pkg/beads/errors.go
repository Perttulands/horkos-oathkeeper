package beads

import (
	"context"
	"errors"
	"strings"
)

// IsCommandUnavailable reports whether err indicates the beads command could
// not be executed.
func IsCommandUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrCommandUnavailable) {
		return true
	}
	msg := normalizeError(err)
	return strings.Contains(msg, "command unavailable")
}

// IsWorkspaceNotInitialized reports whether err indicates the beads workspace
// is missing initialization state.
func IsWorkspaceNotInitialized(err error) bool {
	if err == nil {
		return false
	}
	msg := normalizeError(err)
	patterns := []string{
		"not_initialized",
		"not initialized",
		"beads not initialized",
		"run 'br init'",
		"run: br init",
		"issue_prefix config is missing",
	}
	for _, pattern := range patterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}

// IsTimeoutError reports whether err indicates command timeout/deadline.
func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := normalizeError(err)
	return strings.Contains(msg, "timed out") || strings.Contains(msg, "deadline exceeded")
}

// IsIssueNotFound reports whether err indicates a bead/issue lookup failure.
func IsIssueNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrBeadNotFound) {
		return true
	}
	msg := normalizeError(err)
	patterns := []string{
		"issue_not_found",
		"bead not found",
		"commitment not found",
		"issue not found",
	}
	for _, pattern := range patterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}

func normalizeError(err error) string {
	return strings.ToLower(strings.TrimSpace(err.Error()))
}
