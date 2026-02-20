package recheck

import (
	"fmt"
	"sync"
	"time"
)

// DefaultRecheckInterval is the default interval between re-checks
const DefaultRecheckInterval = 5 * time.Minute

// DefaultMaxAlerts is the default maximum alerts per commitment
const DefaultMaxAlerts = 3

// Commitment status constants
const (
	StatusUnverified = "unverified"
	StatusBacked     = "backed"
	StatusAlerted    = "alerted"
	StatusExpired    = "expired"
)

// TrackedCommitment represents a commitment that needs periodic re-checking
type TrackedCommitment struct {
	ID         string
	Status     string
	DetectedAt time.Time
	ExpiresAt  *time.Time
	AlertCount int
}

// UpdateRequest describes the state change to apply to a commitment
type UpdateRequest struct {
	CommitmentID   string
	NewStatus      string
	Mechanisms     []string
	LastChecked    time.Time
	IncrementAlert bool
}

// FetchFunc retrieves commitments that need re-checking (status unverified or alerted)
type FetchFunc func() ([]TrackedCommitment, error)

// VerifyFunc runs mechanism verification for a commitment detected at the given time.
// Returns (isBacked, mechanisms, error).
type VerifyFunc func(detectedAt time.Time) (bool, []string, error)

// UpdateFunc persists the re-check result for a commitment
type UpdateFunc func(req UpdateRequest) error

// AlertFunc sends an alert for an unbacked commitment
type AlertFunc func(c TrackedCommitment) error

// ErrorFunc receives non-fatal runtime errors encountered during re-checking.
type ErrorFunc func(err error)

// Config holds the configuration for the Rechecker
type Config struct {
	Interval   time.Duration
	MaxAlerts  int
	FetchFunc  FetchFunc
	VerifyFunc VerifyFunc
	UpdateFunc UpdateFunc
	AlertFunc  AlertFunc
	ErrorFunc  ErrorFunc
}

// Rechecker periodically re-checks unresolved commitments
type Rechecker struct {
	config Config
	stopCh chan struct{}
	once   sync.Once
}

func (r *Rechecker) reportError(err error) {
	if err == nil || r.config.ErrorFunc == nil {
		return
	}
	r.config.ErrorFunc(err)
}

// New creates a Rechecker with the given configuration
func New(config Config) *Rechecker {
	if config.Interval == 0 {
		config.Interval = DefaultRecheckInterval
	}
	if config.MaxAlerts == 0 {
		config.MaxAlerts = DefaultMaxAlerts
	}
	return &Rechecker{
		config: config,
		stopCh: make(chan struct{}),
	}
}

// RunOnce performs a single re-check cycle across all pending commitments
func (r *Rechecker) RunOnce() {
	commitments, err := r.config.FetchFunc()
	if err != nil {
		r.reportError(fmt.Errorf("fetch commitments: %w", err))
		return
	}

	// REASON: Recheck is intentionally best-effort per commitment. We report
	// errors via ErrorFunc and continue so one failing record does not block the rest.
	now := time.Now()

	for _, c := range commitments {
		// Check expiration before verification
		if c.ExpiresAt != nil && c.ExpiresAt.Before(now) {
			if err := r.config.UpdateFunc(UpdateRequest{
				CommitmentID: c.ID,
				NewStatus:    StatusExpired,
				LastChecked:  now,
			}); err != nil {
				r.reportError(fmt.Errorf("update expired commitment %s: %w", c.ID, err))
			}
			continue
		}

		// Run verification
		isBacked, mechanisms, err := r.config.VerifyFunc(c.DetectedAt)
		if err != nil {
			r.reportError(fmt.Errorf("verify commitment %s: %w", c.ID, err))
			// Verification error — still update last_checked but treat as unbacked
			isBacked = false
			mechanisms = nil
		}

		if isBacked {
			if err := r.config.UpdateFunc(UpdateRequest{
				CommitmentID: c.ID,
				NewStatus:    StatusBacked,
				Mechanisms:   mechanisms,
				LastChecked:  now,
			}); err != nil {
				r.reportError(fmt.Errorf("update backed commitment %s: %w", c.ID, err))
			}
			continue
		}

		// Not backed — decide whether to alert
		shouldAlert := c.AlertCount < r.config.MaxAlerts
		newStatus := StatusAlerted
		incrementAlert := false

		if shouldAlert {
			if r.config.AlertFunc == nil {
				r.reportError(fmt.Errorf("alert function is nil for commitment %s", c.ID))
				newStatus = c.Status
			} else if err := r.config.AlertFunc(c); err != nil {
				r.reportError(fmt.Errorf("alert commitment %s: %w", c.ID, err))
				newStatus = c.Status
			} else {
				incrementAlert = true
			}
		}

		if err := r.config.UpdateFunc(UpdateRequest{
			CommitmentID:   c.ID,
			NewStatus:      newStatus,
			LastChecked:    now,
			IncrementAlert: incrementAlert,
		}); err != nil {
			r.reportError(fmt.Errorf("update unbacked commitment %s: %w", c.ID, err))
		}
	}
}

// Start begins periodic re-checking in a goroutine
func (r *Rechecker) Start() {
	go func() {
		ticker := time.NewTicker(r.config.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				r.RunOnce()
			case <-r.stopCh:
				return
			}
		}
	}()
}

// Stop halts the periodic re-checker
func (r *Rechecker) Stop() {
	r.once.Do(func() {
		close(r.stopCh)
	})
}
