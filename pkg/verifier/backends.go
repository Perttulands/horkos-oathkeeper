package verifier

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Options configures verifier backends.
type Options struct {
	CronAPIURL   string
	CronEndpoint string
	BeadsCommand string
	StateDirs    []string
	MemoryDirs   []string
}

// NewVerifierFromConfig creates a verifier with all configured backends.
func NewVerifierFromConfig(opts Options) *Verifier {
	checkers := []Checker{
		NewCronCheckerWithEndpoint(opts.CronAPIURL, opts.CronEndpoint),
	}

	if strings.TrimSpace(opts.BeadsCommand) != "" {
		checkers = append(checkers, NewBeadChecker(opts.BeadsCommand))
	}
	if len(opts.StateDirs) > 0 {
		checkers = append(checkers, NewFileChecker("state_files", opts.StateDirs))
	}
	if len(opts.MemoryDirs) > 0 {
		checkers = append(checkers, NewFileChecker("memory_files", opts.MemoryDirs))
	}

	return &Verifier{checkers: checkers}
}

// FileChecker detects recent file writes in configured directories.
type FileChecker struct {
	name        string
	dirs        []string
	maxFindings int
}

// NewFileChecker creates a directory-backed mechanism checker.
func NewFileChecker(name string, dirs []string) *FileChecker {
	return &FileChecker{
		name:        name,
		dirs:        append([]string{}, dirs...),
		maxFindings: 25,
	}
}

// Name returns checker name for reporting.
func (c *FileChecker) Name() string {
	return c.name
}

// Check finds files modified at or after detectedAt.
func (c *FileChecker) Check(detectedAt time.Time) ([]string, error) {
	mechanisms := make([]string, 0, 4)

	for _, dir := range c.dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}

		walkErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return nil
			}
			info, statErr := d.Info()
			if statErr != nil {
				return nil
			}
			if info.ModTime().Before(detectedAt) {
				return nil
			}
			mechanisms = append(mechanisms, fmt.Sprintf("file:%s", path))
			if len(mechanisms) >= c.maxFindings {
				return io.EOF
			}
			return nil
		})
		if walkErr != nil && walkErr != io.EOF {
			return nil, fmt.Errorf("scan %s: %w", dir, walkErr)
		}
		if len(mechanisms) >= c.maxFindings {
			break
		}
	}

	return mechanisms, nil
}

// BeadChecker queries open oathkeeper beads as a verification mechanism.
type BeadChecker struct {
	command string
	timeout time.Duration
}

// NewBeadChecker creates a checker using the configured beads command.
func NewBeadChecker(command string) *BeadChecker {
	command = strings.TrimSpace(command)
	if command == "" {
		command = "bd"
	}
	return &BeadChecker{
		command: command,
		timeout: 5 * time.Second,
	}
}

// Name returns checker name for reporting.
func (c *BeadChecker) Name() string {
	return "beads"
}

// SetTimeout overrides command timeout.
func (c *BeadChecker) SetTimeout(d time.Duration) {
	if d > 0 {
		c.timeout = d
	}
}

// Check lists open oathkeeper beads and reports recent ones as mechanisms.
func (c *BeadChecker) Check(detectedAt time.Time) ([]string, error) {
	if _, err := exec.LookPath(c.command); err != nil {
		return nil, fmt.Errorf("beads command unavailable: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.command, "list", "--json", "--label", "oathkeeper", "--status", "open")
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("beads list timeout: %w", ctx.Err())
		}
		return nil, fmt.Errorf("beads list failed: %w", err)
	}

	type beadItem struct {
		ID         string `json:"id"`
		CreatedAt  string `json:"created_at"`
		CreatedAt2 string `json:"createdAt"`
	}
	var items []beadItem
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("parse beads json: %w", err)
	}

	mechanisms := make([]string, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		created := parseBeadCreatedAt(item.CreatedAt, item.CreatedAt2)
		if !created.IsZero() && created.Before(detectedAt) {
			continue
		}
		mechanisms = append(mechanisms, fmt.Sprintf("bead:%s", id))
	}
	return mechanisms, nil
}

func parseBeadCreatedAt(values ...string) time.Time {
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			return t
		}
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			return t
		}
	}
	return time.Time{}
}
