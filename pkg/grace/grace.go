package grace

import (
	"sync"
	"time"
)

// DefaultGracePeriod is the default wait time before verifying a commitment
const DefaultGracePeriod = 30 * time.Second

// VerifyFunc is the function signature for running verification.
// It receives the time the commitment was detected and returns an outcome.
type VerifyFunc func(detectedAt time.Time) (*VerificationOutcome, error)

// VerificationOutcome contains the result of post-grace-period verification
type VerificationOutcome struct {
	CommitmentID string
	IsBacked     bool
	Mechanisms   []string
}

// pendingEntry tracks a scheduled verification
type pendingEntry struct {
	timer    *time.Timer
	cancelCh chan struct{}
}

// GracePeriod schedules verification after a configurable delay.
// It allows the agent time to create backing mechanisms before being alerted.
type GracePeriod struct {
	duration time.Duration
	verifyFn VerifyFunc

	mu      sync.Mutex
	pending map[string]*pendingEntry
}

// New creates a GracePeriod scheduler with the given duration and verify function
func New(duration time.Duration, verifyFn VerifyFunc) *GracePeriod {
	return &GracePeriod{
		duration: duration,
		verifyFn: verifyFn,
		pending:  make(map[string]*pendingEntry),
	}
}

// Schedule queues a commitment for verification after the grace period.
// The callback is called with the verification outcome when complete.
// If a commitment with the same ID is already pending, the old one is cancelled first.
func (gp *GracePeriod) Schedule(commitmentID string, detectedAt time.Time, callback func(VerificationOutcome)) {
	cancelCh := make(chan struct{})

	entry := &pendingEntry{
		cancelCh: cancelCh,
	}

	// Add entry to pending before starting the timer to avoid a race where
	// the timer fires before the entry is registered.
	gp.mu.Lock()
	if old, exists := gp.pending[commitmentID]; exists {
		close(old.cancelCh)
		old.timer.Stop()
	}
	gp.pending[commitmentID] = entry
	gp.mu.Unlock()

	entry.timer = time.AfterFunc(gp.duration, func() {
		// Check if cancelled before running verification
		select {
		case <-cancelCh:
			return
		default:
		}

		var (
			outcome *VerificationOutcome
			err     error
		)
		if gp.verifyFn != nil {
			outcome, err = gp.verifyFn(detectedAt)
		}
		if err != nil {
			outcome = &VerificationOutcome{
				CommitmentID: commitmentID,
				IsBacked:     false,
				Mechanisms:   []string{},
			}
		}
		if outcome == nil {
			outcome = &VerificationOutcome{
				CommitmentID: commitmentID,
				IsBacked:     false,
				Mechanisms:   []string{},
			}
		}
		outcome.CommitmentID = commitmentID

		gp.mu.Lock()
		delete(gp.pending, commitmentID)
		gp.mu.Unlock()

		if callback != nil {
			callback(*outcome)
		}
	})
}

// Cancel cancels a pending verification. Returns true if the commitment was pending.
func (gp *GracePeriod) Cancel(commitmentID string) bool {
	gp.mu.Lock()
	entry, exists := gp.pending[commitmentID]
	if exists {
		delete(gp.pending, commitmentID)
	}
	gp.mu.Unlock()

	if !exists {
		return false
	}

	close(entry.cancelCh)
	entry.timer.Stop()
	return true
}

// Stop cancels all pending verifications
func (gp *GracePeriod) Stop() {
	gp.mu.Lock()
	entries := make(map[string]*pendingEntry)
	for k, v := range gp.pending {
		entries[k] = v
	}
	gp.pending = make(map[string]*pendingEntry)
	gp.mu.Unlock()

	for _, entry := range entries {
		close(entry.cancelCh)
		entry.timer.Stop()
	}
}

// Pending returns the number of commitments waiting for verification
func (gp *GracePeriod) Pending() int {
	gp.mu.Lock()
	defer gp.mu.Unlock()
	return len(gp.pending)
}
