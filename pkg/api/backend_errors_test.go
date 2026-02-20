package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/perttulands/oathkeeper/pkg/beads"
)

func TestMapBackendError(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantPart   string
		wantHint   bool
	}{
		{
			name:       "not found",
			err:        beads.ErrBeadNotFound,
			wantStatus: http.StatusNotFound,
			wantPart:   "commitment not found",
			wantHint:   false,
		},
		{
			name:       "workspace not initialized",
			err:        errors.New("Beads not initialized: run 'br init'"),
			wantStatus: http.StatusServiceUnavailable,
			wantPart:   "not initialized",
			wantHint:   true,
		},
		{
			name:       "command unavailable",
			err:        errors.New("beads command unavailable"),
			wantStatus: http.StatusServiceUnavailable,
			wantPart:   "command unavailable",
			wantHint:   true,
		},
		{
			name:       "timeout",
			err:        context.DeadlineExceeded,
			wantStatus: http.StatusGatewayTimeout,
			wantPart:   "timed out",
			wantHint:   true,
		},
		{
			name:       "generic",
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
			wantPart:   "boom",
			wantHint:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapBackendError("list commitments", tc.err)
			if got.Status != tc.wantStatus {
				t.Fatalf("status = %d, want %d", got.Status, tc.wantStatus)
			}
			if !strings.Contains(strings.ToLower(got.Msg), strings.ToLower(tc.wantPart)) {
				t.Fatalf("message %q does not contain %q", got.Msg, tc.wantPart)
			}
			if tc.wantHint && strings.TrimSpace(got.Hint) == "" {
				t.Fatal("expected non-empty hint")
			}
			if !tc.wantHint && got.Hint != "" {
				t.Fatalf("expected empty hint, got %q", got.Hint)
			}
		})
	}
}
