package recheck

import (
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

// Config holds the configuration for the Rechecker
type Config struct {
	Interval   time.Duration
	MaxAlerts  int
	FetchFunc  FetchFunc
	VerifyFunc VerifyFunc
	UpdateFunc UpdateFunc
	AlertFunc  AlertFunc
}

// Rechecker periodically re-checks unresolved commitments
type Rechecker struct {
	config Config
	stopCh chan struct{}
	once   sync.Once
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
		return
	}

	now := time.Now()

	for _, c := range commitments {
		// Check expiration before verification
		if c.ExpiresAt != nil && c.ExpiresAt.Before(now) {
			r.config.UpdateFunc(UpdateRequest{
				CommitmentID: c.ID,
				NewStatus:    StatusExpired,
				LastChecked:  now,
			})
			continue
		}

		// Run verification
		isBacked, mechanisms, err := r.config.VerifyFunc(c.DetectedAt)
		if err != nil {
			// Verification error — still update last_checked but treat as unbacked
			isBacked = false
			mechanisms = nil
		}

		if isBacked {
			r.config.UpdateFunc(UpdateRequest{
				CommitmentID: c.ID,
				NewStatus:    StatusBacked,
				Mechanisms:   mechanisms,
				LastChecked:  now,
			})
			continue
		}

		// Not backed — decide whether to alert
		shouldAlert := c.AlertCount < r.config.MaxAlerts
		if shouldAlert {
			r.config.AlertFunc(c)
		}

		r.config.UpdateFunc(UpdateRequest{
			CommitmentID:   c.ID,
			NewStatus:      StatusAlerted,
			LastChecked:    now,
			IncrementAlert: shouldAlert,
		})
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
