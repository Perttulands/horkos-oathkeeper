package beadtracker

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// BeadTracker creates tracking beads for unresolved commitments via the br command.
type BeadTracker struct {
	command string
	timeout time.Duration
}

// NewBeadTracker creates a tracker that shells out to the given beads command.
func NewBeadTracker(command string) *BeadTracker {
	return &BeadTracker{
		command: command,
		timeout: 5 * time.Second,
	}
}

// SetTimeout sets the execution timeout for bead creation.
func (bt *BeadTracker) SetTimeout(d time.Duration) {
	bt.timeout = d
}

// CreateBead creates a tracking bead for a commitment and returns the bead ID.
func (bt *BeadTracker) CreateBead(commitmentID, text, category string, detectedAt time.Time, expiresAt *time.Time) (string, error) {
	title := bt.beadTitle(text)
	body := bt.beadBody(commitmentID, text, category, detectedAt, expiresAt)

	args := []string{"create", "--title", title, "--body", body, "--tag", "oathkeeper"}

	ctx, cancel := context.WithTimeout(context.Background(), bt.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bt.command, args...)
	cmd.WaitDelay = time.Second

	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("br create timed out: %w", ctx.Err())
		}
		return "", fmt.Errorf("br create failed: %w", err)
	}

	beadID := strings.TrimSpace(string(out))
	if beadID == "" {
		return "", fmt.Errorf("br create returned empty output")
	}

	return beadID, nil
}

// beadTitle generates the bead title from commitment text, truncated to fit.
func (bt *BeadTracker) beadTitle(text string) string {
	const prefix = "oathkeeper: "
	const maxLen = 120

	truncated := text
	maxTextLen := maxLen - len(prefix)
	if len(truncated) > maxTextLen {
		truncated = truncated[:maxTextLen-3] + "..."
	}

	return prefix + truncated
}

// beadBody generates the bead body with commitment details.
func (bt *BeadTracker) beadBody(commitmentID, text, category string, detectedAt time.Time, expiresAt *time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Commitment ID: %s\n", commitmentID)
	fmt.Fprintf(&b, "Text: %s\n", text)
	fmt.Fprintf(&b, "Category: %s\n", category)
	fmt.Fprintf(&b, "Detected: %s\n", detectedAt.Format(time.RFC3339))
	if expiresAt != nil {
		fmt.Fprintf(&b, "Expires: %s\n", expiresAt.Format(time.RFC3339))
	}
	return b.String()
}
