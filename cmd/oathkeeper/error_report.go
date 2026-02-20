package main

import (
	"strings"

	"github.com/perttulands/oathkeeper/pkg/beads"
)

type cliErrorReport struct {
	Message string
	Detail  string
	Hint    string
}

func buildCLIErrorReport(message string, detail error) cliErrorReport {
	report := cliErrorReport{
		Message: strings.TrimSpace(message),
	}
	if detail == nil {
		return report
	}

	report.Detail = strings.TrimSpace(detail.Error())
	if report.Detail == report.Message {
		report.Detail = ""
	}
	report.Hint = classifyCLIErrorHint(detail)
	return report
}

func classifyCLIErrorHint(err error) string {
	switch {
	case beads.IsIssueNotFound(err):
		return "Verify the bead ID with `oathkeeper list` and retry."
	case beads.IsWorkspaceNotInitialized(err):
		return "Beads workspace is not initialized. Ask a human to run `br init` in the target workspace."
	case beads.IsCommandUnavailable(err):
		return "Install/configure the beads CLI (br/bd) and set `verification.beads_command` if needed."
	case beads.IsTimeoutError(err):
		return "Backend command timed out. Retry or increase relevant timeout settings."
	default:
		return ""
	}
}
