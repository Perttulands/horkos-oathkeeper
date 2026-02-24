package relaypub

import (
	"testing"
	"time"
)

func TestNewUnbackedEventProducesValidSchema(t *testing.T) {
	event := NewUnbackedEvent("oathkeeper", "br-123", "I'll check that", "temporal", time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC))
	if err := event.Validate(); err != nil {
		t.Fatalf("expected valid event, got %v", err)
	}
}

func TestNewResolvedEventProducesValidSchema(t *testing.T) {
	event := NewResolvedEvent("oathkeeper", "br-123", "done", time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC))
	if err := event.Validate(); err != nil {
		t.Fatalf("expected valid event, got %v", err)
	}
}

func TestRelayEventValidateRejectsUnsupportedEvent(t *testing.T) {
	event := RelayEvent{
		Type:      EventTypeAlert,
		Event:     EventName("unknown.event"),
		Source:    "oathkeeper",
		Timestamp: "2026-02-20T12:00:00Z",
		BeadID:    "br-123",
	}
	if err := event.Validate(); err == nil {
		t.Fatal("expected validation failure for unsupported event")
	}
}

func TestNewUnbackedEventWithContextCarriesCorrelationFields(t *testing.T) {
	event := NewUnbackedEventWithContext(
		"oathkeeper",
		"br-321",
		"I'll check this",
		"followup",
		"sess-77",
		"v2-sess-77-12345",
		time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC),
	)

	if err := event.Validate(); err != nil {
		t.Fatalf("expected valid event, got %v", err)
	}
	if event.SessionKey != "sess-77" {
		t.Fatalf("expected session key sess-77, got %q", event.SessionKey)
	}
	if event.CommitmentID != "v2-sess-77-12345" {
		t.Fatalf("expected commitment id v2-sess-77-12345, got %q", event.CommitmentID)
	}
}
