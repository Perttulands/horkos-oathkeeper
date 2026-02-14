package recheck

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockCommitment creates a test commitment with sensible defaults
func mockCommitment(id, status string) TrackedCommitment {
	return TrackedCommitment{
		ID:         id,
		Status:     status,
		DetectedAt: time.Now().Add(-5 * time.Minute),
		AlertCount: 0,
	}
}

func TestRecheckFindsBackedCommitment(t *testing.T) {
	commitment := mockCommitment("c1", StatusUnverified)

	var updated []UpdateRequest
	var mu sync.Mutex

	r := New(Config{
		Interval:  50 * time.Millisecond,
		MaxAlerts: 3,
		FetchFunc: func() ([]TrackedCommitment, error) {
			return []TrackedCommitment{commitment}, nil
		},
		VerifyFunc: func(detectedAt time.Time) (bool, []string, error) {
			return true, []string{"cron:abc123"}, nil
		},
		UpdateFunc: func(req UpdateRequest) error {
			mu.Lock()
			updated = append(updated, req)
			mu.Unlock()
			return nil
		},
		AlertFunc: func(c TrackedCommitment) error {
			t.Error("alert should not be sent for backed commitment")
			return nil
		},
	})

	r.RunOnce()

	mu.Lock()
	defer mu.Unlock()
	if len(updated) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updated))
	}
	if updated[0].NewStatus != StatusBacked {
		t.Errorf("expected status backed, got %s", updated[0].NewStatus)
	}
	if len(updated[0].Mechanisms) != 1 || updated[0].Mechanisms[0] != "cron:abc123" {
		t.Errorf("expected mechanisms [cron:abc123], got %v", updated[0].Mechanisms)
	}
}

func TestRecheckAlertsUnbackedCommitment(t *testing.T) {
	commitment := mockCommitment("c2", StatusUnverified)

	var alerted []string
	var updated []UpdateRequest
	var mu sync.Mutex

	r := New(Config{
		Interval:  50 * time.Millisecond,
		MaxAlerts: 3,
		FetchFunc: func() ([]TrackedCommitment, error) {
			return []TrackedCommitment{commitment}, nil
		},
		VerifyFunc: func(detectedAt time.Time) (bool, []string, error) {
			return false, nil, nil
		},
		UpdateFunc: func(req UpdateRequest) error {
			mu.Lock()
			updated = append(updated, req)
			mu.Unlock()
			return nil
		},
		AlertFunc: func(c TrackedCommitment) error {
			mu.Lock()
			alerted = append(alerted, c.ID)
			mu.Unlock()
			return nil
		},
	})

	r.RunOnce()

	mu.Lock()
	defer mu.Unlock()
	if len(alerted) != 1 || alerted[0] != "c2" {
		t.Errorf("expected alert for c2, got %v", alerted)
	}
	if len(updated) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updated))
	}
	if updated[0].NewStatus != StatusAlerted {
		t.Errorf("expected status alerted, got %s", updated[0].NewStatus)
	}
	if updated[0].IncrementAlert != true {
		t.Error("expected alert count to be incremented")
	}
}

func TestRecheckExpiresOldCommitment(t *testing.T) {
	expiredTime := time.Now().Add(-1 * time.Hour)
	commitment := TrackedCommitment{
		ID:         "c3",
		Status:     StatusUnverified,
		DetectedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt:  &expiredTime,
		AlertCount: 0,
	}

	var updated []UpdateRequest
	var mu sync.Mutex

	r := New(Config{
		Interval:  50 * time.Millisecond,
		MaxAlerts: 3,
		FetchFunc: func() ([]TrackedCommitment, error) {
			return []TrackedCommitment{commitment}, nil
		},
		VerifyFunc: func(detectedAt time.Time) (bool, []string, error) {
			return false, nil, nil
		},
		UpdateFunc: func(req UpdateRequest) error {
			mu.Lock()
			updated = append(updated, req)
			mu.Unlock()
			return nil
		},
		AlertFunc: func(c TrackedCommitment) error {
			t.Error("alert should not be sent for expired commitment")
			return nil
		},
	})

	r.RunOnce()

	mu.Lock()
	defer mu.Unlock()
	if len(updated) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updated))
	}
	if updated[0].NewStatus != StatusExpired {
		t.Errorf("expected status expired, got %s", updated[0].NewStatus)
	}
}

func TestRecheckSkipsAlertWhenMaxReached(t *testing.T) {
	commitment := TrackedCommitment{
		ID:         "c4",
		Status:     StatusAlerted,
		DetectedAt: time.Now().Add(-30 * time.Minute),
		AlertCount: 3,
	}

	alertCalled := false
	var updated []UpdateRequest
	var mu sync.Mutex

	r := New(Config{
		Interval:  50 * time.Millisecond,
		MaxAlerts: 3,
		FetchFunc: func() ([]TrackedCommitment, error) {
			return []TrackedCommitment{commitment}, nil
		},
		VerifyFunc: func(detectedAt time.Time) (bool, []string, error) {
			return false, nil, nil
		},
		UpdateFunc: func(req UpdateRequest) error {
			mu.Lock()
			updated = append(updated, req)
			mu.Unlock()
			return nil
		},
		AlertFunc: func(c TrackedCommitment) error {
			mu.Lock()
			alertCalled = true
			mu.Unlock()
			return nil
		},
	})

	r.RunOnce()

	mu.Lock()
	defer mu.Unlock()
	if alertCalled {
		t.Error("alert should not be sent when max alerts reached")
	}
	// Should still update last_checked
	if len(updated) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updated))
	}
	if updated[0].IncrementAlert {
		t.Error("should not increment alert count when max reached")
	}
}

func TestRecheckMultipleCommitments(t *testing.T) {
	expiredTime := time.Now().Add(-1 * time.Hour)
	commitments := []TrackedCommitment{
		mockCommitment("backed1", StatusUnverified),
		mockCommitment("alert1", StatusAlerted),
		{
			ID:         "expired1",
			Status:     StatusUnverified,
			DetectedAt: time.Now().Add(-2 * time.Hour),
			ExpiresAt:  &expiredTime,
		},
	}

	var updated []UpdateRequest
	var mu sync.Mutex

	r := New(Config{
		Interval:  50 * time.Millisecond,
		MaxAlerts: 3,
		FetchFunc: func() ([]TrackedCommitment, error) {
			return commitments, nil
		},
		VerifyFunc: func(detectedAt time.Time) (bool, []string, error) {
			// Only the first commitment (backed1) has a mechanism
			if detectedAt.Equal(commitments[0].DetectedAt) {
				return true, []string{"bead:watcher"}, nil
			}
			return false, nil, nil
		},
		UpdateFunc: func(req UpdateRequest) error {
			mu.Lock()
			updated = append(updated, req)
			mu.Unlock()
			return nil
		},
		AlertFunc: func(c TrackedCommitment) error {
			return nil
		},
	})

	r.RunOnce()

	mu.Lock()
	defer mu.Unlock()
	if len(updated) != 3 {
		t.Fatalf("expected 3 updates, got %d", len(updated))
	}

	// Find updates by ID
	statusByID := make(map[string]string)
	for _, u := range updated {
		statusByID[u.CommitmentID] = u.NewStatus
	}

	if statusByID["backed1"] != StatusBacked {
		t.Errorf("backed1: expected backed, got %s", statusByID["backed1"])
	}
	if statusByID["alert1"] != StatusAlerted {
		t.Errorf("alert1: expected alerted, got %s", statusByID["alert1"])
	}
	if statusByID["expired1"] != StatusExpired {
		t.Errorf("expired1: expected expired, got %s", statusByID["expired1"])
	}
}

func TestRecheckUpdatesLastChecked(t *testing.T) {
	commitment := mockCommitment("c5", StatusUnverified)

	var updated []UpdateRequest
	var mu sync.Mutex
	before := time.Now()

	r := New(Config{
		Interval:  50 * time.Millisecond,
		MaxAlerts: 3,
		FetchFunc: func() ([]TrackedCommitment, error) {
			return []TrackedCommitment{commitment}, nil
		},
		VerifyFunc: func(detectedAt time.Time) (bool, []string, error) {
			return true, []string{"cron:x"}, nil
		},
		UpdateFunc: func(req UpdateRequest) error {
			mu.Lock()
			updated = append(updated, req)
			mu.Unlock()
			return nil
		},
		AlertFunc: func(c TrackedCommitment) error {
			return nil
		},
	})

	r.RunOnce()

	mu.Lock()
	defer mu.Unlock()
	if len(updated) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updated))
	}
	if updated[0].LastChecked.Before(before) {
		t.Error("last_checked should be set to current time")
	}
}

func TestRecheckPeriodicRunning(t *testing.T) {
	var callCount int
	var mu sync.Mutex

	r := New(Config{
		Interval:  30 * time.Millisecond,
		MaxAlerts: 3,
		FetchFunc: func() ([]TrackedCommitment, error) {
			mu.Lock()
			callCount++
			mu.Unlock()
			return nil, nil
		},
		VerifyFunc: func(detectedAt time.Time) (bool, []string, error) {
			return false, nil, nil
		},
		UpdateFunc: func(req UpdateRequest) error { return nil },
		AlertFunc:  func(c TrackedCommitment) error { return nil },
	})

	r.Start()
	time.Sleep(100 * time.Millisecond)
	r.Stop()

	mu.Lock()
	count := callCount
	mu.Unlock()

	if count < 2 {
		t.Errorf("expected at least 2 fetch calls, got %d", count)
	}
}

func TestRecheckStopIsClean(t *testing.T) {
	r := New(Config{
		Interval:  50 * time.Millisecond,
		MaxAlerts: 3,
		FetchFunc: func() ([]TrackedCommitment, error) {
			return nil, nil
		},
		VerifyFunc: func(detectedAt time.Time) (bool, []string, error) {
			return false, nil, nil
		},
		UpdateFunc: func(req UpdateRequest) error { return nil },
		AlertFunc:  func(c TrackedCommitment) error { return nil },
	})

	r.Start()
	r.Stop()
	// Double stop should not panic
	r.Stop()
}

func TestRecheckFetchErrorDoesNotPanic(t *testing.T) {
	r := New(Config{
		Interval:  50 * time.Millisecond,
		MaxAlerts: 3,
		FetchFunc: func() ([]TrackedCommitment, error) {
			return nil, fmt.Errorf("database connection failed")
		},
		VerifyFunc: func(detectedAt time.Time) (bool, []string, error) {
			return false, nil, nil
		},
		UpdateFunc: func(req UpdateRequest) error { return nil },
		AlertFunc:  func(c TrackedCommitment) error { return nil },
	})

	// Should not panic
	r.RunOnce()
}

func TestRecheckVerifyErrorContinues(t *testing.T) {
	commitments := []TrackedCommitment{
		mockCommitment("fail1", StatusUnverified),
		mockCommitment("ok1", StatusUnverified),
	}

	var updated []UpdateRequest
	var mu sync.Mutex
	callNum := 0

	r := New(Config{
		Interval:  50 * time.Millisecond,
		MaxAlerts: 3,
		FetchFunc: func() ([]TrackedCommitment, error) {
			return commitments, nil
		},
		VerifyFunc: func(detectedAt time.Time) (bool, []string, error) {
			callNum++
			if callNum == 1 {
				return false, nil, fmt.Errorf("verify error")
			}
			return true, []string{"cron:y"}, nil
		},
		UpdateFunc: func(req UpdateRequest) error {
			mu.Lock()
			updated = append(updated, req)
			mu.Unlock()
			return nil
		},
		AlertFunc: func(c TrackedCommitment) error {
			return nil
		},
	})

	r.RunOnce()

	mu.Lock()
	defer mu.Unlock()
	// The second commitment should still be processed even though first verify failed
	if len(updated) < 1 {
		t.Fatal("expected at least 1 update despite verify error")
	}

	// Find the ok1 update
	found := false
	for _, u := range updated {
		if u.CommitmentID == "ok1" && u.NewStatus == StatusBacked {
			found = true
		}
	}
	if !found {
		t.Error("ok1 should be updated to backed despite fail1 verify error")
	}
}

func TestRecheckExpirationCheckBeforeVerify(t *testing.T) {
	// An expired commitment should not trigger verify at all
	expiredTime := time.Now().Add(-1 * time.Hour)
	commitment := TrackedCommitment{
		ID:         "expired-no-verify",
		Status:     StatusUnverified,
		DetectedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt:  &expiredTime,
	}

	verifyCalled := false

	r := New(Config{
		Interval:  50 * time.Millisecond,
		MaxAlerts: 3,
		FetchFunc: func() ([]TrackedCommitment, error) {
			return []TrackedCommitment{commitment}, nil
		},
		VerifyFunc: func(detectedAt time.Time) (bool, []string, error) {
			verifyCalled = true
			return false, nil, nil
		},
		UpdateFunc: func(req UpdateRequest) error { return nil },
		AlertFunc:  func(c TrackedCommitment) error { return nil },
	})

	r.RunOnce()

	if verifyCalled {
		t.Error("verify should not be called for expired commitments")
	}
}

func TestRecheckDefaultInterval(t *testing.T) {
	if DefaultRecheckInterval != 5*time.Minute {
		t.Errorf("expected default interval 5m, got %v", DefaultRecheckInterval)
	}
}

func TestRecheckAlertFailureDoesNotConsumeAlert(t *testing.T) {
	commitment := mockCommitment("c-alert-fail", StatusUnverified)

	var updated []UpdateRequest
	var mu sync.Mutex

	r := New(Config{
		Interval:  50 * time.Millisecond,
		MaxAlerts: 3,
		FetchFunc: func() ([]TrackedCommitment, error) {
			return []TrackedCommitment{commitment}, nil
		},
		VerifyFunc: func(detectedAt time.Time) (bool, []string, error) {
			return false, nil, nil
		},
		UpdateFunc: func(req UpdateRequest) error {
			mu.Lock()
			updated = append(updated, req)
			mu.Unlock()
			return nil
		},
		AlertFunc: func(c TrackedCommitment) error {
			return fmt.Errorf("alert transport failed")
		},
	})

	r.RunOnce()

	mu.Lock()
	defer mu.Unlock()
	if len(updated) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updated))
	}
	if updated[0].NewStatus != StatusUnverified {
		t.Fatalf("expected status %q when alert fails, got %q", StatusUnverified, updated[0].NewStatus)
	}
	if updated[0].IncrementAlert {
		t.Fatal("expected IncrementAlert=false when alert send fails")
	}
}

func TestRecheckReportsUpdateErrors(t *testing.T) {
	commitment := mockCommitment("c-update-fail", StatusUnverified)

	var reported []error
	var mu sync.Mutex

	r := New(Config{
		Interval:  50 * time.Millisecond,
		MaxAlerts: 3,
		FetchFunc: func() ([]TrackedCommitment, error) {
			return []TrackedCommitment{commitment}, nil
		},
		VerifyFunc: func(detectedAt time.Time) (bool, []string, error) {
			return true, []string{"cron:ok"}, nil
		},
		UpdateFunc: func(req UpdateRequest) error {
			return fmt.Errorf("storage unavailable")
		},
		AlertFunc: func(c TrackedCommitment) error {
			return nil
		},
		ErrorFunc: func(err error) {
			mu.Lock()
			reported = append(reported, err)
			mu.Unlock()
		},
	})

	r.RunOnce()

	mu.Lock()
	defer mu.Unlock()
	if len(reported) == 0 {
		t.Fatal("expected update error to be reported")
	}
	if reported[0] == nil || reported[0].Error() == "" {
		t.Fatal("expected non-empty reported error")
	}
}
