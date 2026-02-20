package relaypub

import (
	"testing"
	"time"
)

func TestNewUnbackedEventProducesValidSchema(t *testing.T) {
	event := NewUnbackedEvent("oathkeeper", "bd-123", "I'll check that", "temporal", time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC))
	if err := event.Validate(); err != nil {
		t.Fatalf("expected valid event, got %v", err)
	}
}

func TestNewResolvedEventProducesValidSchema(t *testing.T) {
	event := NewResolvedEvent("oathkeeper", "bd-123", "done", time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC))
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
		BeadID:    "bd-123",
	}
	if err := event.Validate(); err == nil {
		t.Fatal("expected validation failure for unsupported event")
	}
}
