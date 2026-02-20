package beads

import (
	"context"
	"errors"
	"testing"
)

func TestIsCommandUnavailable(t *testing.T) {
	err := errors.New("wrapped: beads command unavailable: br")
	if !IsCommandUnavailable(err) {
		t.Fatal("expected IsCommandUnavailable to detect text pattern")
	}

	wrapped := errors.New("x")
	if IsCommandUnavailable(wrapped) {
		t.Fatal("unexpected command unavailable classification")
	}
}

func TestIsWorkspaceNotInitialized(t *testing.T) {
	cases := []error{
		errors.New("Beads not initialized: run 'br init' first"),
		errors.New(`{"code":"NOT_INITIALIZED","hint":"Run: br init"}`),
		errors.New("database not initialized: issue_prefix config is missing"),
	}
	for _, err := range cases {
		if !IsWorkspaceNotInitialized(err) {
			t.Fatalf("expected workspace-not-initialized classification for %q", err.Error())
		}
	}
}

func TestIsTimeoutError(t *testing.T) {
	if !IsTimeoutError(context.DeadlineExceeded) {
		t.Fatal("expected deadline exceeded classification")
	}
	if !IsTimeoutError(errors.New("br list timed out: context deadline exceeded")) {
		t.Fatal("expected timed out classification")
	}
	if IsTimeoutError(errors.New("generic failure")) {
		t.Fatal("unexpected timeout classification")
	}
}

func TestIsIssueNotFound(t *testing.T) {
	if !IsIssueNotFound(ErrBeadNotFound) {
		t.Fatal("expected sentinel not found classification")
	}
	if !IsIssueNotFound(errors.New("Issue not found: athena-ab3")) {
		t.Fatal("expected issue not found classification")
	}
	if !IsIssueNotFound(errors.New(`{"code":"ISSUE_NOT_FOUND"}`)) {
		t.Fatal("expected ISSUE_NOT_FOUND classification")
	}
	if IsIssueNotFound(errors.New("query failed")) {
		t.Fatal("unexpected not found classification")
	}
}
