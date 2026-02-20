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
	Retries int
}

// Publisher publishes Oathkeeper events to Relay.
type Publisher struct {
	cfg Config
	run commandRunner
}

type commandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

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
	if cfg.Retries <= 0 {
		cfg.Retries = 1
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
	return p.NotifyUnbackedWithContext(beadID, text, category, "", "")
}

// NotifyUnbackedWithContext publishes a commitment.unbacked event with
// optional session/commitment correlation metadata.
func (p *Publisher) NotifyUnbackedWithContext(beadID, text, category, sessionKey, commitmentID string) error {
	return p.publish(
		NewUnbackedEventWithContext(p.cfg.From, beadID, text, category, sessionKey, commitmentID, time.Now()),
		beadID,
		"high",
		"oathkeeper,commitment,alert",
	)
}

// NotifyResolved publishes a commitment.resolved event.
func (p *Publisher) NotifyResolved(beadID, evidence string) error {
	return p.publish(
		NewResolvedEvent(p.cfg.From, beadID, evidence, time.Now()),
		beadID,
		"normal",
		"oathkeeper,commitment,resolved",
	)
}

func (p *Publisher) publish(payload RelayEvent, thread, priority, tags string) error {
	if !p.Enabled() {
		return nil
	}
	if strings.TrimSpace(payload.BeadID) == "" {
		payload.BeadID = "unknown"
	}
	if strings.TrimSpace(thread) == "" {
		thread = payload.BeadID
	}
	if err := payload.Validate(); err != nil {
		return fmt.Errorf("invalid relay event payload: %w", err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal relay payload: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= p.cfg.Retries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), p.cfg.Timeout)
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
		cancel()
		if err == nil {
			return nil
		}

		msg := strings.TrimSpace(string(out))
		if msg != "" {
			lastErr = fmt.Errorf("relay send failed: %w (%s)", err, msg)
		} else {
			lastErr = fmt.Errorf("relay send failed: %w", err)
		}
		if attempt < p.cfg.Retries {
			time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
		}
	}
	return lastErr
}

func defaultRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
