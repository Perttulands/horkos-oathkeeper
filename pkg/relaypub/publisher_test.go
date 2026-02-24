package relaypub

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNotifyUnbackedDisabledNoop(t *testing.T) {
	p := New(Config{Enabled: false})
	calls := 0
	p.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		calls++
		return nil, nil
	}

	if err := p.NotifyUnbacked("br-1", "missing backup plan", "followup"); err != nil {
		t.Fatalf("NotifyUnbacked returned error for disabled publisher: %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected no relay command calls, got %d", calls)
	}
}

func TestNotifyUnbackedPublishesRelayMessage(t *testing.T) {
	p := New(Config{
		Enabled: true,
		Command: "relay-test",
		To:      "athena",
		From:    "oathkeeper",
		Timeout: time.Second,
	})

	var gotName string
	var gotArgs []string
	p.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		gotName = name
		gotArgs = append([]string{}, args...)
		return []byte("ok"), nil
	}

	if err := p.NotifyUnbacked("br-42", "I will add tests", "will_do"); err != nil {
		t.Fatalf("NotifyUnbacked returned error: %v", err)
	}
	if gotName != "relay-test" {
		t.Fatalf("expected relay command relay-test, got %q", gotName)
	}
	if len(gotArgs) < 3 {
		t.Fatalf("expected relay args, got %v", gotArgs)
	}
	if gotArgs[0] != "send" || gotArgs[1] != "athena" {
		t.Fatalf("expected send athena prefix, got %v", gotArgs[:2])
	}

	var payload RelayEvent
	if err := json.Unmarshal([]byte(gotArgs[2]), &payload); err != nil {
		t.Fatalf("payload is not valid json: %v", err)
	}
	if payload.Event != EventCommitmentUnbacked {
		t.Fatalf("expected commitment.unbacked event, got %q", payload.Event)
	}
	if payload.BeadID != "br-42" {
		t.Fatalf("expected bead id br-42, got %q", payload.BeadID)
	}
	if payload.Category != "will_do" {
		t.Fatalf("expected category will_do, got %q", payload.Category)
	}
}

func TestNotifyUnbackedWithContextIncludesCorrelationMetadata(t *testing.T) {
	p := New(Config{
		Enabled: true,
		Command: "relay-test",
		To:      "athena",
		From:    "oathkeeper",
		Timeout: time.Second,
	})

	var gotArgs []string
	p.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		gotArgs = append([]string{}, args...)
		return []byte("ok"), nil
	}

	if err := p.NotifyUnbackedWithContext("br-52", "missing backup plan", "followup", "session-a", "v2-session-a-1"); err != nil {
		t.Fatalf("NotifyUnbackedWithContext returned error: %v", err)
	}

	var payload RelayEvent
	if err := json.Unmarshal([]byte(gotArgs[2]), &payload); err != nil {
		t.Fatalf("payload is not valid json: %v", err)
	}
	if payload.SessionKey != "session-a" {
		t.Fatalf("expected session key session-a, got %q", payload.SessionKey)
	}
	if payload.CommitmentID != "v2-session-a-1" {
		t.Fatalf("expected commitment id v2-session-a-1, got %q", payload.CommitmentID)
	}
}

func TestNotifyResolvedIncludesRunnerOutputOnError(t *testing.T) {
	p := New(Config{
		Enabled: true,
		Command: "relay",
		To:      "athena",
		From:    "oathkeeper",
		Timeout: time.Second,
	})
	p.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("relay unavailable"), errors.New("exit status 1")
	}

	err := p.NotifyResolved("br-99", "closed by merge")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "relay unavailable") {
		t.Fatalf("expected runner output in error, got %q", msg)
	}
}

func TestNotifyResolvedRetriesThenSucceeds(t *testing.T) {
	p := New(Config{
		Enabled: true,
		Command: "relay",
		To:      "athena",
		From:    "oathkeeper",
		Timeout: time.Second,
		Retries: 3,
	})

	calls := 0
	p.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		calls++
		if calls < 2 {
			return []byte("temporary failure"), errors.New("exit status 1")
		}
		return []byte("ok"), nil
	}

	if err := p.NotifyResolved("br-77", "done"); err != nil {
		t.Fatalf("expected retry success, got error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 attempts, got %d", calls)
	}
}

func TestNotifyUnbackedExhaustsRetries(t *testing.T) {
	p := New(Config{
		Enabled: true,
		Command: "relay",
		To:      "athena",
		From:    "oathkeeper",
		Timeout: time.Second,
		Retries: 3,
	})

	calls := 0
	p.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		calls++
		return []byte("relay unavailable"), errors.New("exit status 1")
	}

	err := p.NotifyUnbacked("br-11", "I'll do it", "followup")
	if err == nil {
		t.Fatal("expected retry exhaustion error")
	}
	if calls != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls)
	}
}

func TestPublishRejectsSchemaMismatch(t *testing.T) {
	p := New(Config{Enabled: true, Command: "relay", To: "athena", From: "oathkeeper", Timeout: time.Second})
	p.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("ok"), nil
	}

	err := p.publish(RelayEvent{
		Type:      EventTypeResolution,
		Event:     EventCommitmentUnbacked,
		Source:    "oathkeeper",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		BeadID:    "br-1",
	}, "br-1", "normal", "oathkeeper")
	if err == nil {
		t.Fatal("expected schema validation error, got nil")
	}
	if !strings.Contains(err.Error(), "requires type=alert") {
		t.Fatalf("unexpected error: %v", err)
	}
}
