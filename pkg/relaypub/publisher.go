package relaypub

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Config controls Relay publishing behavior.
type Config struct {
	Enabled bool
	Command string
	To      string
	From    string
	Timeout time.Duration
}

// Publisher publishes Oathkeeper events to Relay.
type Publisher struct {
	cfg Config
	run commandRunner
}

type commandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

type eventPayload struct {
	Type      string `json:"type"`
	Event     string `json:"event"`
	Source    string `json:"source"`
	Timestamp string `json:"timestamp"`
	BeadID    string `json:"bead_id"`
	Text      string `json:"text,omitempty"`
	Category  string `json:"category,omitempty"`
	Evidence  string `json:"evidence,omitempty"`
}

// New creates a Relay publisher.
func New(cfg Config) *Publisher {
	if strings.TrimSpace(cfg.Command) == "" {
		cfg.Command = "relay"
	}
	if strings.TrimSpace(cfg.To) == "" {
		cfg.To = "athena"
	}
	if strings.TrimSpace(cfg.From) == "" {
		cfg.From = "oathkeeper"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	return &Publisher{
		cfg: cfg,
		run: defaultRunner,
	}
}

// Enabled reports whether publishing is active.
func (p *Publisher) Enabled() bool {
	return p != nil && p.cfg.Enabled
}

// NotifyUnbacked publishes a commitment.unbacked event.
func (p *Publisher) NotifyUnbacked(beadID, text, category string) error {
	return p.publish(
		eventPayload{
			Type:      "alert",
			Event:     "commitment.unbacked",
			Source:    p.cfg.From,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			BeadID:    beadID,
			Text:      text,
			Category:  category,
		},
		beadID,
		"high",
		"oathkeeper,commitment,alert",
	)
}

// NotifyResolved publishes a commitment.resolved event.
func (p *Publisher) NotifyResolved(beadID, evidence string) error {
	return p.publish(
		eventPayload{
			Type:      "resolution",
			Event:     "commitment.resolved",
			Source:    p.cfg.From,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			BeadID:    beadID,
			Evidence:  evidence,
		},
		beadID,
		"normal",
		"oathkeeper,commitment,resolved",
	)
}

func (p *Publisher) publish(payload eventPayload, thread, priority, tags string) error {
	if !p.Enabled() {
		return nil
	}
	if strings.TrimSpace(payload.BeadID) == "" {
		payload.BeadID = "unknown"
	}
	if strings.TrimSpace(thread) == "" {
		thread = payload.BeadID
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal relay payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.Timeout)
	defer cancel()

	out, err := p.run(
		ctx,
		p.cfg.Command,
		"send",
		p.cfg.To,
		string(raw),
		"--agent",
		p.cfg.From,
		"--thread",
		thread,
		"--priority",
		priority,
		"--tag",
		tags,
	)
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("relay send failed: %w (%s)", err, msg)
		}
		return fmt.Errorf("relay send failed: %w", err)
	}
	return nil
}

func defaultRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
