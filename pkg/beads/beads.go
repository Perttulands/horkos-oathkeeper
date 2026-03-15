package beads

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"
)

// ErrCommandUnavailable indicates the configured br command could not be executed.
var ErrCommandUnavailable = errors.New("beads command unavailable")

// ErrBeadNotFound indicates the requested bead could not be found.
var ErrBeadNotFound = errors.New("bead not found")

// CommitmentInfo carries fields needed to create oathkeeper tracking beads.
type CommitmentInfo struct {
	Text       string
	Category   string
	Tags       []string
	SessionKey string
	DetectedAt time.Time
	ExpiresAt  time.Time
}

// Filter controls bead query options for BeadStore.List.
type Filter struct {
	Status   string
	Category string
	Since    time.Time
}

// Bead is a normalized bead representation from br CLI JSON responses.
type Bead struct {
	ID          string
	Title       string
	Status      string
	Tags        []string
	CreatedAt   time.Time
	ClosedAt    time.Time
	CloseReason string
}

// BeadStore wraps br CLI calls for bead lifecycle management.
type BeadStore struct {
	command string
	timeout time.Duration
	dryRun  bool
	seq     uint64
}

// NewBeadStore creates a BeadStore for the given command.
func NewBeadStore(command string) *BeadStore {
	if strings.TrimSpace(command) == "" {
		command = "br"
	}
	return &BeadStore{
		command: command,
		timeout: 5 * time.Second,
	}
}

// SetTimeout sets the CLI execution timeout.
func (bs *BeadStore) SetTimeout(d time.Duration) {
	bs.timeout = d
}

// SetDryRun enables or disables dry-run mode. In dry-run mode, mutating
// operations (create/close/resolve) are simulated and no CLI command is run.
func (bs *BeadStore) SetDryRun(enabled bool) {
	bs.dryRun = enabled
}

// Create creates a bead and returns its bead ID.
func (bs *BeadStore) Create(commitment CommitmentInfo) (string, error) {
	title := "oathkeeper: " + strings.TrimSpace(commitment.Text)
	if title == "oathkeeper:" {
		title = "oathkeeper: (empty commitment)"
	}
	if bs.dryRun {
		return bs.syntheticBeadID(title), nil
	}

	tags := createTags(commitment)
	args := buildCreateArgs(title, tags)

	out, err := bs.run(args...)
	if err != nil {
		return "", fmt.Errorf("create bead: %w", err)
	}

	id := strings.TrimSpace(string(out))
	if id == "" {
		return "", fmt.Errorf("br create returned empty output")
	}
	return id, nil
}

// Close closes an existing bead with an optional reason.
func (bs *BeadStore) Close(beadID string, reason string) error {
	if strings.TrimSpace(beadID) == "" {
		return fmt.Errorf("bead ID cannot be empty")
	}
	if bs.dryRun {
		return nil
	}

	args := []string{"close", beadID}
	if strings.TrimSpace(reason) != "" {
		args = append(args, "--reason", reason)
	}

	_, err := bs.run(args...)
	return err
}

// List returns oathkeeper beads matching the filter.
func (bs *BeadStore) List(filter Filter) ([]Bead, error) {
	listArgs := bs.buildListArgs(filter)
	out, err := bs.run(listArgs...)
	if err != nil {
		return nil, fmt.Errorf("list beads: %w", err)
	}

	beads, err := parseBeadListJSON(out)
	if err != nil {
		return nil, fmt.Errorf("parse bead list: %w", err)
	}

	return filterBeads(beads, filter), nil
}

// Get returns one bead by ID.
func (bs *BeadStore) Get(beadID string) (Bead, error) {
	if strings.TrimSpace(beadID) == "" {
		return Bead{}, fmt.Errorf("bead ID cannot be empty")
	}

	out, err := bs.run("show", beadID, "--json")
	if err != nil {
		return Bead{}, fmt.Errorf("get bead %s: %w", beadID, err)
	}

	beads, err := parseBeadListJSON(out)
	if err != nil {
		return Bead{}, fmt.Errorf("parse bead response: %w", err)
	}

	for _, bead := range beads {
		if bead.ID == beadID {
			return bead, nil
		}
	}
	return Bead{}, ErrBeadNotFound
}

func (bs *BeadStore) buildListArgs(filter Filter) []string {
	args := []string{"list"}

	status := strings.ToLower(strings.TrimSpace(filter.Status))
	if status == "closed" {
		args = append(args, "--all", "--status", "closed")
	} else if status != "" {
		args = append(args, "--status", status)
	} else {
		// No status filter: include all beads (open + closed)
		args = append(args, "--all")
	}

	// v2 list does not support --label; filter client-side in filterBeads.
	args = append(args, "--json")

	return args
}

func (bs *BeadStore) run(args ...string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("br: no subcommand specified")
	}

	if _, err := exec.LookPath(bs.command); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrCommandUnavailable, bs.command)
	}

	ctx, cancel := context.WithTimeout(context.Background(), bs.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bs.command, args...)
	cmd.WaitDelay = time.Second

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("br %s timed out: %w", args[0], ctx.Err())
		}
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("br %s failed: %w: %s", args[0], err, errMsg)
		}
		return nil, fmt.Errorf("br %s failed: %w", args[0], err)
	}
	return out, nil
}

func filterBeads(beads []Bead, filter Filter) []Bead {
	category := strings.TrimSpace(filter.Category)
	filtered := make([]Bead, 0, len(beads))
	for _, bead := range beads {
		// Always require "oathkeeper" label (was server-side in v1, now client-side)
		if !containsTag(bead.Tags, "oathkeeper") {
			continue
		}
		if !filter.Since.IsZero() && bead.CreatedAt.Before(filter.Since) {
			continue
		}
		if category != "" && !containsTag(bead.Tags, category) {
			continue
		}
		filtered = append(filtered, bead)
	}
	return filtered
}

func containsTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}

func createTags(commitment CommitmentInfo) []string {
	tags := []string{"oathkeeper"}
	if category := strings.TrimSpace(commitment.Category); category != "" {
		tags = append(tags, category)
	}
	for _, tag := range commitment.Tags {
		if normalized := strings.TrimSpace(tag); normalized != "" {
			tags = append(tags, normalized)
		}
	}
	if session := sessionTag(commitment.SessionKey); session != "" {
		tags = append(tags, session)
	}
	return uniqueStrings(tags)
}

func buildCreateArgs(title string, tags []string) []string {
	args := []string{
		"create",
		"--title", title,
		"--priority", "2",
	}
	if len(tags) > 0 {
		args = append(args, "--labels", strings.Join(tags, ","))
	}
	args = append(args, "--silent")
	return args
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func sessionTag(sessionKey string) string {
	normalized := strings.TrimSpace(strings.ToLower(sessionKey))
	if normalized == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(normalized))
	for _, r := range normalized {
		isAlphaNum := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if isAlphaNum || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}

	key := strings.Trim(b.String(), "-")
	if key == "" {
		return ""
	}
	return "session-" + key
}

type beadJSON struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Status      string   `json:"status"`
	Tags        []string `json:"tags"`
	Labels      []string `json:"labels"`
	CloseReason string   `json:"close_reason"`
	CreatedAt   string   `json:"created_at"`
	CreatedAt2  string   `json:"createdAt"`
	ClosedAt    string   `json:"closed_at"`
	ClosedAt2   string   `json:"closedAt"`
}

func parseBeadListJSON(payload []byte) ([]Bead, error) {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("br returned empty JSON output")
	}

	var list []beadJSON
	if err := json.Unmarshal(trimmed, &list); err != nil {
		var single beadJSON
		if err2 := json.Unmarshal(trimmed, &single); err2 != nil {
			return nil, fmt.Errorf("parse br JSON output as list (%v) and single object (%w)", err, err2)
		}
		list = []beadJSON{single}
	}

	beads := make([]Bead, 0, len(list))
	for _, item := range list {
		bead, err := normalizeBead(item)
		if err != nil {
			return nil, fmt.Errorf("normalize bead: %w", err)
		}
		beads = append(beads, bead)
	}
	return beads, nil
}

func normalizeBead(item beadJSON) (Bead, error) {
	created, err := parseJSONTime(firstNonEmpty(item.CreatedAt, item.CreatedAt2))
	if err != nil {
		return Bead{}, fmt.Errorf("parse created_at: %w", err)
	}

	closed, err := parseJSONTimeAllowEmpty(firstNonEmpty(item.ClosedAt, item.ClosedAt2))
	if err != nil {
		return Bead{}, fmt.Errorf("parse closed_at: %w", err)
	}

	tags := item.Tags
	if len(tags) == 0 {
		tags = item.Labels
	}

	return Bead{
		ID:          item.ID,
		Title:       item.Title,
		Status:      item.Status,
		Tags:        tags,
		CreatedAt:   created,
		ClosedAt:    closed,
		CloseReason: item.CloseReason,
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parseJSONTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}

	t, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return t, nil
	}
	t, err = time.Parse(time.RFC3339, value)
	if err == nil {
		return t, nil
	}
	return time.Time{}, err
}

func parseJSONTimeAllowEmpty(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	return parseJSONTime(value)
}

func (bs *BeadStore) syntheticBeadID(seed string) string {
	n := atomic.AddUint64(&bs.seq, 1)
	sum := sha1.Sum([]byte(fmt.Sprintf("%s-%d", strings.TrimSpace(seed), n)))
	return fmt.Sprintf("dryrun-%x", sum[:4])
}
