package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/perttulands/oathkeeper/pkg/beads"
)

func TestBuildCLIErrorReportWithoutDetail(t *testing.T) {
	report := buildCLIErrorReport("Could not list commitments.", nil)
	if report.Message != "Could not list commitments." {
		t.Fatalf("unexpected message: %q", report.Message)
	}
	if report.Detail != "" {
		t.Fatalf("expected empty detail, got %q", report.Detail)
	}
	if report.Hint != "" {
		t.Fatalf("expected empty hint, got %q", report.Hint)
	}
}

func TestBuildCLIErrorReportWithClassifiedHints(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		hintPart string
	}{
		{
			name:     "workspace not initialized",
			err:      errors.New("Beads not initialized: run 'br init' first"),
			hintPart: "run `br init`",
		},
		{
			name:     "command unavailable",
			err:      errors.New("beads command unavailable"),
			hintPart: "beads CLI",
		},
		{
			name:     "timeout",
			err:      context.DeadlineExceeded,
			hintPart: "timed out",
		},
		{
			name:     "not found",
			err:      beads.ErrBeadNotFound,
			hintPart: "bead ID",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := buildCLIErrorReport("Operation failed.", tc.err)
			if report.Detail == "" {
				t.Fatal("expected detail to be populated")
			}
			if report.Hint == "" {
				t.Fatal("expected hint to be populated")
			}
			if !strings.Contains(report.Hint, tc.hintPart) {
				t.Fatalf("hint %q does not contain expected %q", report.Hint, tc.hintPart)
			}
		})
	}
}

func TestBuildCLIErrorReportSuppressesDuplicateDetail(t *testing.T) {
	err := errors.New("same message")
	report := buildCLIErrorReport("same message", err)
	if report.Detail != "" {
		t.Fatalf("expected duplicate detail to be suppressed, got %q", report.Detail)
	}
}
