package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/perttulands/oathkeeper/pkg/beads"
)

type backendError struct {
	Status int
	Msg    string
	Hint   string
}

func mapBackendError(operation string, err error) backendError {
	op := strings.TrimSpace(operation)
	if op == "" {
		op = "beads backend operation"
	}

	switch {
	case beads.IsIssueNotFound(err):
		return backendError{
			Status: http.StatusNotFound,
			Msg:    "commitment not found",
		}
	case beads.IsWorkspaceNotInitialized(err):
		return backendError{
			Status: http.StatusServiceUnavailable,
			Msg:    fmt.Sprintf("%s failed: beads workspace not initialized", op),
			Hint:   "A human must run `br init` in the target workspace before Oathkeeper can query commitments.",
		}
	case beads.IsCommandUnavailable(err):
		return backendError{
			Status: http.StatusServiceUnavailable,
			Msg:    fmt.Sprintf("%s failed: beads command unavailable", op),
			Hint:   "Install/configure the beads CLI (br) and set verification.beads_command accordingly.",
		}
	case beads.IsTimeoutError(err):
		return backendError{
			Status: http.StatusGatewayTimeout,
			Msg:    fmt.Sprintf("%s failed: beads command timed out", op),
			Hint:   "Retry the request or increase command timeout settings.",
		}
	default:
		return backendError{
			Status: http.StatusInternalServerError,
			Msg:    fmt.Sprintf("%s: %v", op, err),
		}
	}
}
