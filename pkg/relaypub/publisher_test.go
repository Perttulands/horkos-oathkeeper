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

	if err := p.NotifyUnbacked("bd-1", "missing backup plan", "followup"); err != nil {
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

	if err := p.NotifyUnbacked("bd-42", "I will add tests", "will_do"); err != nil {
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

	var payload eventPayload
	if err := json.Unmarshal([]byte(gotArgs[2]), &payload); err != nil {
		t.Fatalf("payload is not valid json: %v", err)
	}
	if payload.Event != "commitment.unbacked" {
		t.Fatalf("expected commitment.unbacked event, got %q", payload.Event)
	}
	if payload.BeadID != "bd-42" {
		t.Fatalf("expected bead id bd-42, got %q", payload.BeadID)
	}
	if payload.Category != "will_do" {
		t.Fatalf("expected category will_do, got %q", payload.Category)
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

	err := p.NotifyResolved("bd-99", "closed by merge")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "relay unavailable") {
		t.Fatalf("expected runner output in error, got %q", msg)
	}
}
