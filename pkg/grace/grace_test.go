package grace

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockChecker implements verifier-like behavior for testing
type mockChecker struct {
	mu         sync.Mutex
	mechanisms []string
	err        error
}

func (m *mockChecker) setResult(mechanisms []string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mechanisms = mechanisms
	m.err = err
}

func (m *mockChecker) getResult() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mechanisms, m.err
}

func TestGracePeriodSchedulesVerification(t *testing.T) {
	resultCh := make(chan VerificationOutcome, 1)

	checker := &mockChecker{mechanisms: []string{"cron:abc123"}}
	verifyFn := func(detectedAt time.Time) (*VerificationOutcome, error) {
		mechs, _ := checker.getResult()
		return &VerificationOutcome{
			IsBacked:   len(mechs) > 0,
			Mechanisms: mechs,
		}, nil
	}

	gp := New(10*time.Millisecond, verifyFn)
	defer gp.Stop()

	gp.Schedule("commit-1", time.Now(), func(outcome VerificationOutcome) {
		resultCh <- outcome
	})

	select {
	case result := <-resultCh:
		if !result.IsBacked {
			t.Error("expected commitment to be backed")
		}
		if len(result.Mechanisms) != 1 || result.Mechanisms[0] != "cron:abc123" {
			t.Errorf("expected [cron:abc123], got %v", result.Mechanisms)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for verification result")
	}
}

func TestGracePeriodWaitsBeforeVerifying(t *testing.T) {
	scheduledAt := time.Now()
	var verifiedAt time.Time
	resultCh := make(chan VerificationOutcome, 1)

	verifyFn := func(detectedAt time.Time) (*VerificationOutcome, error) {
		verifiedAt = time.Now()
		return &VerificationOutcome{IsBacked: false, Mechanisms: []string{}}, nil
	}

	gracePeriod := 50 * time.Millisecond
	gp := New(gracePeriod, verifyFn)
	defer gp.Stop()

	gp.Schedule("commit-2", time.Now(), func(outcome VerificationOutcome) {
		resultCh <- outcome
	})

	select {
	case <-resultCh:
		elapsed := verifiedAt.Sub(scheduledAt)
		if elapsed < gracePeriod {
			t.Errorf("verification ran too early: elapsed %v, expected at least %v", elapsed, gracePeriod)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for verification result")
	}
}

func TestGracePeriodNotBacked(t *testing.T) {
	resultCh := make(chan VerificationOutcome, 1)

	verifyFn := func(detectedAt time.Time) (*VerificationOutcome, error) {
		return &VerificationOutcome{IsBacked: false, Mechanisms: []string{}}, nil
	}

	gp := New(10*time.Millisecond, verifyFn)
	defer gp.Stop()

	gp.Schedule("commit-3", time.Now(), func(outcome VerificationOutcome) {
		resultCh <- outcome
	})

	select {
	case result := <-resultCh:
		if result.IsBacked {
			t.Error("expected commitment to NOT be backed")
		}
		if len(result.Mechanisms) != 0 {
			t.Errorf("expected no mechanisms, got %v", result.Mechanisms)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for verification result")
	}
}

func TestGracePeriodMultipleCommitments(t *testing.T) {
	results := make(chan VerificationOutcome, 3)

	callCount := 0
	var mu sync.Mutex

	verifyFn := func(detectedAt time.Time) (*VerificationOutcome, error) {
		mu.Lock()
		callCount++
		count := callCount
		mu.Unlock()

		backed := count%2 == 1 // odd calls are backed
		var mechs []string
		if backed {
			mechs = []string{"cron:test"}
		}
		return &VerificationOutcome{IsBacked: backed, Mechanisms: mechs}, nil
	}

	gp := New(10*time.Millisecond, verifyFn)
	defer gp.Stop()

	for i := 0; i < 3; i++ {
		gp.Schedule("commit-multi-"+string(rune('a'+i)), time.Now(), func(outcome VerificationOutcome) {
			results <- outcome
		})
	}

	for i := 0; i < 3; i++ {
		select {
		case <-results:
			// received result
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for result %d", i+1)
		}
	}
}

func TestGracePeriodPassesDetectedAt(t *testing.T) {
	resultCh := make(chan VerificationOutcome, 1)
	expectedTime := time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC)
	var receivedTime time.Time

	verifyFn := func(detectedAt time.Time) (*VerificationOutcome, error) {
		receivedTime = detectedAt
		return &VerificationOutcome{IsBacked: false, Mechanisms: []string{}}, nil
	}

	gp := New(10*time.Millisecond, verifyFn)
	defer gp.Stop()

	gp.Schedule("commit-time", expectedTime, func(outcome VerificationOutcome) {
		resultCh <- outcome
	})

	select {
	case <-resultCh:
		if !receivedTime.Equal(expectedTime) {
			t.Errorf("expected detectedAt %v, got %v", expectedTime, receivedTime)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out")
	}
}

func TestGracePeriodCancel(t *testing.T) {
	verifyFn := func(detectedAt time.Time) (*VerificationOutcome, error) {
		return &VerificationOutcome{IsBacked: false, Mechanisms: []string{}}, nil
	}

	gp := New(100*time.Millisecond, verifyFn)

	called := false
	gp.Schedule("commit-cancel", time.Now(), func(outcome VerificationOutcome) {
		called = true
	})

	// Cancel before grace period expires
	cancelled := gp.Cancel("commit-cancel")
	if !cancelled {
		t.Error("expected Cancel to return true for pending commitment")
	}

	// Wait longer than the grace period
	time.Sleep(200 * time.Millisecond)
	gp.Stop()

	if called {
		t.Error("callback should not have been called after cancellation")
	}
}

func TestGracePeriodCancelNonExistent(t *testing.T) {
	verifyFn := func(detectedAt time.Time) (*VerificationOutcome, error) {
		return &VerificationOutcome{IsBacked: false, Mechanisms: []string{}}, nil
	}

	gp := New(10*time.Millisecond, verifyFn)
	defer gp.Stop()

	cancelled := gp.Cancel("nonexistent")
	if cancelled {
		t.Error("expected Cancel to return false for non-existent commitment")
	}
}

func TestGracePeriodStop(t *testing.T) {
	verifyFn := func(detectedAt time.Time) (*VerificationOutcome, error) {
		return &VerificationOutcome{IsBacked: false, Mechanisms: []string{}}, nil
	}

	gp := New(100*time.Millisecond, verifyFn)

	called := false
	gp.Schedule("commit-stop", time.Now(), func(outcome VerificationOutcome) {
		called = true
	})

	// Stop immediately — should cancel all pending
	gp.Stop()

	time.Sleep(200 * time.Millisecond)

	if called {
		t.Error("callback should not have been called after Stop")
	}
}

func TestGracePeriodPending(t *testing.T) {
	verifyFn := func(detectedAt time.Time) (*VerificationOutcome, error) {
		return &VerificationOutcome{IsBacked: false, Mechanisms: []string{}}, nil
	}

	gp := New(200*time.Millisecond, verifyFn)
	defer gp.Stop()

	if gp.Pending() != 0 {
		t.Errorf("expected 0 pending, got %d", gp.Pending())
	}

	gp.Schedule("commit-p1", time.Now(), func(outcome VerificationOutcome) {})
	gp.Schedule("commit-p2", time.Now(), func(outcome VerificationOutcome) {})

	if gp.Pending() != 2 {
		t.Errorf("expected 2 pending, got %d", gp.Pending())
	}
}

func TestDefaultGracePeriod(t *testing.T) {
	if DefaultGracePeriod != 30*time.Second {
		t.Errorf("expected default grace period of 30s, got %v", DefaultGracePeriod)
	}
}

func TestVerificationOutcomeFields(t *testing.T) {
	outcome := VerificationOutcome{
		CommitmentID: "test-123",
		IsBacked:     true,
		Mechanisms:   []string{"cron:abc", "bead:xyz"},
	}

	if outcome.CommitmentID != "test-123" {
		t.Errorf("expected CommitmentID test-123, got %s", outcome.CommitmentID)
	}
	if !outcome.IsBacked {
		t.Error("expected IsBacked true")
	}
	if len(outcome.Mechanisms) != 2 {
		t.Errorf("expected 2 mechanisms, got %d", len(outcome.Mechanisms))
	}
}

func TestGracePeriodDuplicateSchedule(t *testing.T) {
	resultCh := make(chan VerificationOutcome, 2)

	verifyFn := func(detectedAt time.Time) (*VerificationOutcome, error) {
		return &VerificationOutcome{IsBacked: false, Mechanisms: []string{}}, nil
	}

	gp := New(50*time.Millisecond, verifyFn)
	defer gp.Stop()

	// Schedule the same ID twice — should cancel the first
	gp.Schedule("dup-1", time.Now(), func(outcome VerificationOutcome) {
		resultCh <- outcome
	})
	gp.Schedule("dup-1", time.Now(), func(outcome VerificationOutcome) {
		resultCh <- outcome
	})

	// Should have exactly 1 pending (not 2)
	if gp.Pending() != 1 {
		t.Errorf("expected 1 pending after duplicate schedule, got %d", gp.Pending())
	}

	// Should only receive one callback
	select {
	case <-resultCh:
		// good
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for verification result")
	}

	// Give enough time to confirm no second callback
	time.Sleep(100 * time.Millisecond)
	select {
	case <-resultCh:
		t.Error("received unexpected second callback from duplicate schedule")
	default:
		// good - no second callback
	}
}

func TestGracePeriodVerifyError(t *testing.T) {
	resultCh := make(chan VerificationOutcome, 1)

	verifyFn := func(detectedAt time.Time) (*VerificationOutcome, error) {
		return nil, fmt.Errorf("verify failed")
	}

	gp := New(10*time.Millisecond, verifyFn)
	defer gp.Stop()

	gp.Schedule("err-1", time.Now(), func(outcome VerificationOutcome) {
		resultCh <- outcome
	})

	select {
	case result := <-resultCh:
		if result.IsBacked {
			t.Error("expected unbacked on verify error")
		}
		if result.CommitmentID != "err-1" {
			t.Errorf("expected commitment ID err-1, got %s", result.CommitmentID)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out")
	}
}
