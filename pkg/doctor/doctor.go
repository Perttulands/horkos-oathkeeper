package doctor

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Status constants for check results.
const (
	StatusPass = "pass"
	StatusFail = "fail"
	StatusWarn = "warn"
)

// CheckResult represents the outcome of a single dependency check.
type CheckResult struct {
	Name     string
	Status   string
	Detail   string
	Required bool
}

// Config holds the parameters for running doctor checks.
type Config struct {
	Version       string
	DBPath        string
	ConfigPath    string
	OpenClawURL   string
	BeadsCommand  string
	TmuxCommand   string
	ClaudeCommand string
	ArgusWebhook  string
}

var httpTimeout = 5 * time.Second

// FormatResult renders a single CheckResult as a human-readable line.
func FormatResult(r CheckResult) string {
	icon := "[✓]"
	switch r.Status {
	case StatusFail:
		icon = "[✗]"
	case StatusWarn:
		icon = "[!]"
	}
	return fmt.Sprintf("%s %s: %s", icon, r.Name, r.Detail)
}

// FormatReport renders all check results as a complete report.
func FormatReport(results []CheckResult) string {
	var b strings.Builder
	pass, fail, warn := 0, 0, 0
	for _, r := range results {
		b.WriteString(FormatResult(r))
		b.WriteString("\n")
		switch r.Status {
		case StatusPass:
			pass++
		case StatusFail:
			fail++
		case StatusWarn:
			warn++
		}
	}
	b.WriteString("\n")

	parts := []string{}
	if pass > 0 {
		parts = append(parts, fmt.Sprintf("%d passed", pass))
	}
	if fail > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", fail))
	}
	if warn > 0 {
		parts = append(parts, fmt.Sprintf("%d warning", warn))
	}
	b.WriteString(strings.Join(parts, ", "))

	if fail == 0 {
		b.WriteString("\n\nAll required dependencies OK.")
	}

	return b.String()
}

// CheckBinary verifies that a binary is available and runs its version command.
func CheckBinary(name, binary, versionFlag string) CheckResult {
	path, err := exec.LookPath(binary)
	if err != nil {
		return CheckResult{
			Name:     name,
			Status:   StatusFail,
			Detail:   fmt.Sprintf("%s not found in PATH", binary),
			Required: true,
		}
	}

	cmd := exec.Command(path, versionFlag)
	out, err := cmd.Output()
	if err != nil {
		return CheckResult{
			Name:     name,
			Status:   StatusPass,
			Detail:   fmt.Sprintf("%s (found at %s)", binary, path),
			Required: true,
		}
	}

	version := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	return CheckResult{
		Name:     name,
		Status:   StatusPass,
		Detail:   fmt.Sprintf("%s (found)", version),
		Required: true,
	}
}

// CheckHTTPEndpoint verifies that an HTTP endpoint is reachable.
func CheckHTTPEndpoint(name, url string) CheckResult {
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return CheckResult{
			Name:     name,
			Status:   StatusFail,
			Detail:   fmt.Sprintf("%s (unreachable)", url),
			Required: true,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return CheckResult{
			Name:     name,
			Status:   StatusFail,
			Detail:   fmt.Sprintf("%s (HTTP %d)", url, resp.StatusCode),
			Required: true,
		}
	}

	return CheckResult{
		Name:     name,
		Status:   StatusPass,
		Detail:   fmt.Sprintf("%s (reachable)", url),
		Required: true,
	}
}

// CheckFileAccessible verifies that a file exists and is readable.
func CheckFileAccessible(name, path string) CheckResult {
	info, err := os.Stat(path)
	if err != nil {
		return CheckResult{
			Name:     name,
			Status:   StatusFail,
			Detail:   fmt.Sprintf("%s (not found)", path),
			Required: true,
		}
	}

	return CheckResult{
		Name:     name,
		Status:   StatusPass,
		Detail:   fmt.Sprintf("%s (accessible, %d bytes)", path, info.Size()),
		Required: true,
	}
}

func checkVersion(version string) CheckResult {
	return CheckResult{
		Name:     "Oathkeeper binary",
		Status:   StatusPass,
		Detail:   fmt.Sprintf("v%s", version),
		Required: true,
	}
}

func checkArgus(cfg Config) CheckResult {
	if cfg.ArgusWebhook == "" {
		return CheckResult{
			Name:     "Argus bot",
			Status:   StatusWarn,
			Detail:   "not configured (optional)",
			Required: false,
		}
	}

	r := CheckHTTPEndpoint("Argus bot", cfg.ArgusWebhook)
	r.Required = false
	return r
}

// RunChecks executes all doctor checks and returns results.
func RunChecks(cfg Config) []CheckResult {
	results := []CheckResult{
		checkVersion(cfg.Version),
		CheckFileAccessible("SQLite database", cfg.DBPath),
		CheckFileAccessible("Config file", cfg.ConfigPath),
		CheckHTTPEndpoint("OpenClaw API", cfg.OpenClawURL),
		CheckBinary("Beads binary", cfg.BeadsCommand, "--version"),
		CheckBinary("Tmux", cfg.TmuxCommand, "-V"),
		CheckBinary("Claude CLI", cfg.ClaudeCommand, "--version"),
		checkArgus(cfg),
	}
	return results
}
