package relaypub

import (
	"fmt"
	"strings"
	"time"
)

// EventType identifies the broad Relay event group.
type EventType string

const (
	EventTypeAlert      EventType = "alert"
	EventTypeResolution EventType = "resolution"
)

// EventName identifies the specific commitment lifecycle event.
type EventName string

const (
	EventCommitmentUnbacked EventName = "commitment.unbacked"
	EventCommitmentResolved EventName = "commitment.resolved"
)

// RelayEvent defines the Oathkeeper payload schema sent to Relay.
type RelayEvent struct {
	Type      EventType `json:"type"`
	Event     EventName `json:"event"`
	Source    string    `json:"source"`
	Timestamp string    `json:"timestamp"`
	BeadID    string    `json:"bead_id"`
	Text      string    `json:"text,omitempty"`
	Category  string    `json:"category,omitempty"`
	Evidence  string    `json:"evidence,omitempty"`
}

// Validate ensures the event matches Oathkeeper's Relay schema.
func (e RelayEvent) Validate() error {
	if strings.TrimSpace(string(e.Type)) == "" {
		return fmt.Errorf("type is required")
	}
	if strings.TrimSpace(string(e.Event)) == "" {
		return fmt.Errorf("event is required")
	}
	if strings.TrimSpace(e.Source) == "" {
		return fmt.Errorf("source is required")
	}
	if strings.TrimSpace(e.Timestamp) == "" {
		return fmt.Errorf("timestamp is required")
	}
	if strings.TrimSpace(e.BeadID) == "" {
		return fmt.Errorf("bead_id is required")
	}

	switch e.Event {
	case EventCommitmentUnbacked:
		if e.Type != EventTypeAlert {
			return fmt.Errorf("commitment.unbacked requires type=alert")
		}
	case EventCommitmentResolved:
		if e.Type != EventTypeResolution {
			return fmt.Errorf("commitment.resolved requires type=resolution")
		}
	default:
		return fmt.Errorf("unsupported event %q", e.Event)
	}

	return nil
}

// NewUnbackedEvent creates a schema-valid commitment.unbacked Relay event.
func NewUnbackedEvent(source, beadID, text, category string, now time.Time) RelayEvent {
	return RelayEvent{
		Type:      EventTypeAlert,
		Event:     EventCommitmentUnbacked,
		Source:    source,
		Timestamp: now.UTC().Format(time.RFC3339),
		BeadID:    beadID,
		Text:      text,
		Category:  category,
	}
}

// NewResolvedEvent creates a schema-valid commitment.resolved Relay event.
func NewResolvedEvent(source, beadID, evidence string, now time.Time) RelayEvent {
	return RelayEvent{
		Type:      EventTypeResolution,
		Event:     EventCommitmentResolved,
		Source:    source,
		Timestamp: now.UTC().Format(time.RFC3339),
		BeadID:    beadID,
		Evidence:  evidence,
	}
}
