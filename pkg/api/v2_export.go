package api

import (
	"time"

	"github.com/perttulands/horkos-oathkeeper/pkg/beads"
	"github.com/perttulands/horkos-oathkeeper/pkg/detector"
	"github.com/perttulands/horkos-oathkeeper/pkg/grace"
)

// NewV2APIWithFuncs constructs a V2API from individual function closures.
// This is intended for integration/E2E testing where the real BeadStore (br CLI)
// is unavailable and callers provide their own implementations.
func NewV2APIWithFuncs(
	detectCommitment func(string) detector.DetectionResult,
	autoResolve func(sessionKey string, message string) ([]string, error),
	listBeads func(filter beads.Filter) ([]beads.Bead, error),
	getBead func(beadID string) (beads.Bead, error),
	resolveBead func(beadID string, reason string) error,
	scheduleGrace func(commitmentID string, detectedAt time.Time, callback func(grace.VerificationOutcome)),
) *V2API {
	return &V2API{
		detectCommitment: detectCommitment,
		autoResolve:      autoResolve,
		listBeads:        listBeads,
		getBead:          getBead,
		resolveBead:      resolveBead,
		scheduleGrace:    scheduleGrace,
		now:              time.Now,
	}
}
