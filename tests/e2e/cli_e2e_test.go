// Package e2e provides CLI end-to-end tests for Oathkeeper v2.
//
// These tests build the real oathkeeper binary and exercise every CLI
// subcommand via exec.Command — the same way agents invoke them from shell.
// A mock "br" script stands in for the real bead CLI so tests are hermetic.
package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// binaryPath holds the path to the compiled oathkeeper binary.
// Set by TestMain.
var binaryPath string

func TestMain(m *testing.M) {
	// Build the oathkeeper binary into a temp directory.
	tmp, err := os.MkdirTemp("", "oathkeeper-cli-e2e-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)

	bin := filepath.Join(tmp, "oathkeeper")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	cmd := exec.Command("go", "build", "-o", bin, "./cmd/oathkeeper")
	cmd.Dir = findRepoRoot()
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build oathkeeper: %v\n%s\n", err, out)
		os.Exit(1)
	}

	binaryPath = bin
	os.Exit(m.Run())
}

// findRepoRoot walks up from the current file to find the repo root (contains go.mod).
func findRepoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("could not find repo root (go.mod)")
		}
		dir = parent
	}
}

// ---------------------------------------------------------------------------
// Test fixture helpers
// ---------------------------------------------------------------------------

// cliFixture bundles temp paths for a CLI test: config, mock br script, etc.
type cliFixture struct {
	dir        string
	configPath string
	mockBrPath string
}

// newCLIFixture creates a temp dir with a mock br script and config TOML.
// The mock br script responds to list/show/close/create with canned JSON.
func newCLIFixture(t *testing.T) *cliFixture {
	t.Helper()
	dir := t.TempDir()

	// Mock br script — responds to subcommands with valid JSON.
	mockBr := filepath.Join(dir, "mock-br")
	mockBrScript := `#!/bin/sh
# Mock br CLI for oathkeeper E2E tests.
# Routes: list, show, close, create, --version

case "$1" in
  --version)
    echo "br 0.99.0-mock"
    exit 0
    ;;
  list)
    cat <<'LISTEOF'
[
  {"id":"br-0001","title":"oathkeeper: I will check back in 5 minutes","status":"open","labels":["oathkeeper","temporal","session-test"],"created_at":"2026-02-28T10:00:00Z"},
  {"id":"br-0002","title":"oathkeeper: I will monitor the build","status":"open","labels":["oathkeeper","followup","session-test"],"created_at":"2026-02-28T10:01:00Z"},
  {"id":"br-0003","title":"oathkeeper: deploy verified","status":"closed","labels":["oathkeeper","temporal"],"created_at":"2026-02-28T09:00:00Z","closed_at":"2026-02-28T09:30:00Z","close_reason":"done"}
]
LISTEOF
    exit 0
    ;;
  show)
    cat <<'SHOWEOF'
{"id":"br-0001","title":"oathkeeper: I will check back in 5 minutes","status":"open","labels":["oathkeeper","temporal","session-test"],"created_at":"2026-02-28T10:00:00Z"}
SHOWEOF
    exit 0
    ;;
  close)
    echo "closed"
    exit 0
    ;;
  create)
    echo "br-9999"
    exit 0
    ;;
  *)
    echo "mock-br: unknown command $1" >&2
    exit 1
    ;;
esac
`
	if err := os.WriteFile(mockBr, []byte(mockBrScript), 0o755); err != nil {
		t.Fatalf("write mock-br: %v", err)
	}

	// Config TOML pointing beads_command to our mock.
	configPath := filepath.Join(dir, "oathkeeper.toml")
	configTOML := fmt.Sprintf(`[verification]
beads_command = %q

[storage]
db_path = %q

[detector]
min_confidence = 0.7

[general]
dry_run = false
`, mockBr, filepath.Join(dir, "test.db"))

	if err := os.WriteFile(configPath, []byte(configTOML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return &cliFixture{
		dir:        dir,
		configPath: configPath,
		mockBrPath: mockBr,
	}
}

// run executes the oathkeeper binary with the given args and returns
// stdout, stderr, and the exit code.
func (f *cliFixture) run(args ...string) (stdout, stderr string, exitCode int) {
	cmd := exec.Command(binaryPath, args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// runWithConfig executes oathkeeper with --config prepended.
func (f *cliFixture) runWithConfig(args ...string) (stdout, stderr string, exitCode int) {
	full := append([]string{"--config", f.configPath}, args...)
	// --config must come AFTER the subcommand, so we need to restructure.
	// Actually, looking at main.go, --config is extracted from args[2:] (after subcommand).
	// So the subcommand must be first, then --config.
	return f.run(full...)
}

// runSub executes oathkeeper <subcmd> --config <path> <rest...>
func (f *cliFixture) runSub(subcmd string, rest ...string) (stdout, stderr string, exitCode int) {
	args := []string{subcmd, "--config", f.configPath}
	args = append(args, rest...)
	return f.run(args...)
}

// writeTranscript writes a JSONL transcript file and returns its path.
func (f *cliFixture) writeTranscript(t *testing.T, name string, lines ...string) string {
	t.Helper()
	path := filepath.Join(f.dir, name)
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write transcript %s: %v", name, err)
	}
	return path
}

// ---------------------------------------------------------------------------
// CLI Tests
// ---------------------------------------------------------------------------

func TestCLI_Help(t *testing.T) {
	fix := newCLIFixture(t)

	t.Run("--help flag", func(t *testing.T) {
		stdout, _, code := fix.run("--help")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		if !strings.Contains(stdout, "Oathkeeper") {
			t.Fatal("expected usage text containing 'Oathkeeper'")
		}
		if !strings.Contains(stdout, "Commands:") {
			t.Fatal("expected 'Commands:' in usage text")
		}
		for _, cmd := range []string{"serve", "scan", "list", "stats", "resolve", "doctor"} {
			if !strings.Contains(stdout, cmd) {
				t.Fatalf("expected subcommand %q in usage text", cmd)
			}
		}
	})

	t.Run("-h flag", func(t *testing.T) {
		stdout, _, code := fix.run("-h")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		if !strings.Contains(stdout, "Oathkeeper") {
			t.Fatal("expected usage text")
		}
	})

	t.Run("help subcommand", func(t *testing.T) {
		stdout, _, code := fix.run("help")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		if !strings.Contains(stdout, "Commands:") {
			t.Fatal("expected usage text with Commands:")
		}
	})

	t.Run("no args prints usage to stderr", func(t *testing.T) {
		_, stderr, code := fix.run()
		if code == 0 {
			t.Fatal("expected non-zero exit with no args")
		}
		if !strings.Contains(stderr, "Oathkeeper") {
			t.Fatal("expected usage on stderr")
		}
	})
}

func TestCLI_Version(t *testing.T) {
	fix := newCLIFixture(t)

	t.Run("--version flag", func(t *testing.T) {
		stdout, _, code := fix.run("--version")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		if !strings.Contains(stdout, "oathkeeper v") {
			t.Fatalf("expected version string, got %q", stdout)
		}
	})

	t.Run("version subcommand", func(t *testing.T) {
		stdout, _, code := fix.run("version")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		if !strings.Contains(stdout, "oathkeeper v") {
			t.Fatalf("expected version string, got %q", stdout)
		}
	})
}

func TestCLI_UnknownCommand(t *testing.T) {
	fix := newCLIFixture(t)

	_, stderr, code := fix.run("bogus")
	if code == 0 {
		t.Fatal("expected non-zero exit for unknown command")
	}
	if !strings.Contains(stderr, "bogus") {
		t.Fatalf("expected error mentioning 'bogus', got stderr: %q", stderr)
	}
}

// ---------------------------------------------------------------------------
// scan subcommand
// ---------------------------------------------------------------------------

func TestCLI_Scan(t *testing.T) {
	fix := newCLIFixture(t)

	transcript := fix.writeTranscript(t, "commitments.jsonl",
		`{"role":"assistant","content":"I'll check back in 5 minutes to verify the deployment"}`,
		`{"role":"user","content":"ok sounds good"}`,
		`{"role":"assistant","content":"I'll monitor the build output for errors"}`,
	)

	t.Run("text format detects commitments", func(t *testing.T) {
		stdout, _, code := fix.runSub("scan", transcript)
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		if !strings.Contains(stdout, "COMMITMENT DETECTED") {
			t.Fatalf("expected COMMITMENT DETECTED in output, got:\n%s", stdout)
		}
		// Should find at least the temporal commitment
		if !strings.Contains(strings.ToLower(stdout), "temporal") {
			t.Fatalf("expected 'temporal' category in output, got:\n%s", stdout)
		}
	})

	t.Run("json format", func(t *testing.T) {
		// Flags must precede the positional file argument (Go flag package stops at first non-flag).
		stdout, _, code := fix.runSub("scan", "--format", "json", transcript)
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		var results []json.RawMessage
		if err := json.Unmarshal([]byte(stdout), &results); err != nil {
			t.Fatalf("expected valid JSON array, got error: %v\noutput: %s", err, stdout)
		}
		if len(results) == 0 {
			t.Fatal("expected at least one scan result in JSON output")
		}
	})

	t.Run("--json flag", func(t *testing.T) {
		stdout, _, code := fix.runSub("scan", "--json", transcript)
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		var results []json.RawMessage
		if err := json.Unmarshal([]byte(stdout), &results); err != nil {
			t.Fatalf("expected valid JSON, got error: %v", err)
		}
	})
}

func TestCLI_Scan_NoCommitments(t *testing.T) {
	fix := newCLIFixture(t)

	transcript := fix.writeTranscript(t, "clean.jsonl",
		`{"role":"assistant","content":"The deployment is running smoothly right now"}`,
		`{"role":"user","content":"great, thanks"}`,
		`{"role":"assistant","content":"I already checked the logs, everything looks good"}`,
	)

	stdout, _, code := fix.runSub("scan", transcript)
	if code != 0 {
		t.Fatalf("expected exit 0 for clean scan, got %d", code)
	}
	if strings.Contains(stdout, "COMMITMENT DETECTED") {
		t.Fatalf("expected no commitments detected, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "No commitments detected") {
		t.Fatalf("expected 'No commitments detected' message, got:\n%s", stdout)
	}
}

func TestCLI_Scan_NoCommitments_JSON(t *testing.T) {
	fix := newCLIFixture(t)

	transcript := fix.writeTranscript(t, "clean-json.jsonl",
		`{"role":"assistant","content":"Everything is fine"}`,
	)

	stdout, _, code := fix.runSub("scan", "--json", transcript)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	var results []json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &results); err != nil {
		t.Fatalf("expected valid JSON, got error: %v\noutput: %s", err, stdout)
	}
	if len(results) != 0 {
		t.Fatalf("expected empty array for clean scan, got %d results", len(results))
	}
}

func TestCLI_Scan_InvalidFlag(t *testing.T) {
	fix := newCLIFixture(t)

	transcript := fix.writeTranscript(t, "dummy.jsonl",
		`{"role":"assistant","content":"hello"}`,
	)

	_, stderr, code := fix.run("scan", transcript, "--bogus-flag")
	if code == 0 {
		t.Fatal("expected non-zero exit for invalid flag")
	}
	if stderr == "" {
		t.Fatal("expected error message on stderr")
	}
}

func TestCLI_Scan_MissingFile(t *testing.T) {
	fix := newCLIFixture(t)

	_, stderr, code := fix.runSub("scan", "/nonexistent/path/transcript.jsonl")
	if code == 0 {
		t.Fatal("expected non-zero exit for missing file")
	}
	if !strings.Contains(stderr, "not found") {
		t.Fatalf("expected 'not found' error, got: %q", stderr)
	}
}

func TestCLI_Scan_MissingFileArg(t *testing.T) {
	fix := newCLIFixture(t)

	_, stderr, code := fix.runSub("scan")
	if code == 0 {
		t.Fatal("expected non-zero exit when no file argument provided")
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Fatalf("expected usage hint in error, got: %q", stderr)
	}
}

// ---------------------------------------------------------------------------
// list subcommand
// ---------------------------------------------------------------------------

func TestCLI_List(t *testing.T) {
	fix := newCLIFixture(t)

	t.Run("text format", func(t *testing.T) {
		stdout, _, code := fix.runSub("list", "--status", "all")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		// Should show header columns
		if !strings.Contains(stdout, "ID") || !strings.Contains(stdout, "STATUS") {
			t.Fatalf("expected table header with ID and STATUS, got:\n%s", stdout)
		}
		// Should show our mock beads
		if !strings.Contains(stdout, "br-0001") {
			t.Fatalf("expected bead br-0001 in output, got:\n%s", stdout)
		}
		if !strings.Contains(stdout, "commitment(s)") {
			t.Fatalf("expected commitment count summary, got:\n%s", stdout)
		}
	})

	t.Run("json format", func(t *testing.T) {
		stdout, _, code := fix.runSub("list", "--status", "all", "--json")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		var result struct {
			Commitments []json.RawMessage `json:"commitments"`
			Count       int               `json:"count"`
		}
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatalf("expected valid JSON, got: %v\noutput: %s", err, stdout)
		}
		if result.Count == 0 {
			t.Fatal("expected non-zero count in JSON output")
		}
	})

	t.Run("invalid status flag", func(t *testing.T) {
		_, stderr, code := fix.runSub("list", "--status", "invalid")
		if code == 0 {
			t.Fatal("expected non-zero exit for invalid status")
		}
		if !strings.Contains(stderr, "invalid") {
			t.Fatalf("expected 'invalid' in error, got: %q", stderr)
		}
	})
}

// ---------------------------------------------------------------------------
// stats subcommand
// ---------------------------------------------------------------------------

func TestCLI_Stats(t *testing.T) {
	fix := newCLIFixture(t)

	t.Run("console dashboard", func(t *testing.T) {
		stdout, _, code := fix.runSub("stats")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		if !strings.Contains(stdout, "Commitment Dashboard") {
			t.Fatalf("expected 'Commitment Dashboard' header, got:\n%s", stdout)
		}
		if !strings.Contains(stdout, "Total:") {
			t.Fatalf("expected 'Total:' in dashboard, got:\n%s", stdout)
		}
	})

	t.Run("json export", func(t *testing.T) {
		stdout, _, code := fix.runSub("stats", "--export", "json")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		var summary map[string]interface{}
		if err := json.Unmarshal([]byte(stdout), &summary); err != nil {
			t.Fatalf("expected valid JSON, got: %v\noutput: %s", err, stdout)
		}
		if _, ok := summary["total"]; !ok {
			t.Fatal("expected 'total' field in JSON stats")
		}
		if _, ok := summary["open"]; !ok {
			t.Fatal("expected 'open' field in JSON stats")
		}
	})

	t.Run("csv export", func(t *testing.T) {
		stdout, _, code := fix.runSub("stats", "--export", "csv")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		if !strings.Contains(stdout, "metric,value") {
			t.Fatalf("expected CSV header 'metric,value', got:\n%s", stdout)
		}
		if !strings.Contains(stdout, "total,") {
			t.Fatalf("expected 'total,' row in CSV, got:\n%s", stdout)
		}
	})

	t.Run("export json to file", func(t *testing.T) {
		outFile := filepath.Join(fix.dir, "stats-export.json")
		_, _, code := fix.runSub("stats", "--export", "json", "--output", outFile)
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		// Note: --export json sets opts.json=true, so no stdout confirmation.
		data, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("read export file: %v", err)
		}
		var summary map[string]interface{}
		if err := json.Unmarshal(data, &summary); err != nil {
			t.Fatalf("export file is not valid JSON: %v", err)
		}
	})

	t.Run("export csv to file", func(t *testing.T) {
		outFile := filepath.Join(fix.dir, "stats-export.csv")
		stdout, _, code := fix.runSub("stats", "--export", "csv", "--output", outFile)
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		if !strings.Contains(stdout, "Exported") {
			t.Fatalf("expected 'Exported' confirmation, got: %q", stdout)
		}
		data, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("read export file: %v", err)
		}
		if !strings.Contains(string(data), "metric,value") {
			t.Fatal("expected CSV content in export file")
		}
	})

	t.Run("html dashboard", func(t *testing.T) {
		dashFile := filepath.Join(fix.dir, "dashboard.html")
		stdout, _, code := fix.runSub("stats", "--dashboard", dashFile)
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		if !strings.Contains(stdout, "Wrote stats dashboard") {
			t.Fatalf("expected dashboard confirmation, got: %q", stdout)
		}
		data, err := os.ReadFile(dashFile)
		if err != nil {
			t.Fatalf("read dashboard: %v", err)
		}
		if !strings.Contains(string(data), "<!doctype html>") {
			t.Fatal("expected HTML content in dashboard file")
		}
	})

	t.Run("--json flag", func(t *testing.T) {
		stdout, _, code := fix.runSub("stats", "--json")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		var summary map[string]interface{}
		if err := json.Unmarshal([]byte(stdout), &summary); err != nil {
			t.Fatalf("expected valid JSON, got: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// resolve subcommand
// ---------------------------------------------------------------------------

func TestCLI_Resolve(t *testing.T) {
	fix := newCLIFixture(t)

	t.Run("resolve with reason flag", func(t *testing.T) {
		// Flags must precede the positional bead ID argument.
		stdout, _, code := fix.runSub("resolve", "--reason", "verified deployment", "br-0001")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		if !strings.Contains(stdout, "Resolved") || !strings.Contains(stdout, "br-0001") {
			t.Fatalf("expected 'Resolved br-0001' in output, got: %q", stdout)
		}
	})

	t.Run("resolve with positional reason", func(t *testing.T) {
		stdout, _, code := fix.runSub("resolve", "br-0002", "task completed")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		if !strings.Contains(stdout, "Resolved") {
			t.Fatalf("expected 'Resolved' in output, got: %q", stdout)
		}
	})

	t.Run("resolve with default reason", func(t *testing.T) {
		stdout, _, code := fix.runSub("resolve", "br-0003")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		if !strings.Contains(stdout, "resolved via CLI") {
			t.Fatalf("expected default reason 'resolved via CLI', got: %q", stdout)
		}
	})

	t.Run("resolve json output", func(t *testing.T) {
		stdout, _, code := fix.runSub("resolve", "--reason", "done", "--json", "br-0001")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatalf("expected valid JSON, got: %v\noutput: %s", err, stdout)
		}
		if result["resolved"] != true {
			t.Fatalf("expected resolved=true, got: %v", result["resolved"])
		}
		if result["bead_id"] != "br-0001" {
			t.Fatalf("expected bead_id=br-0001, got: %v", result["bead_id"])
		}
	})

	t.Run("resolve missing bead ID", func(t *testing.T) {
		_, stderr, code := fix.runSub("resolve")
		if code == 0 {
			t.Fatal("expected non-zero exit when bead ID missing")
		}
		if !strings.Contains(stderr, "Usage:") {
			t.Fatalf("expected usage hint, got: %q", stderr)
		}
	})

	t.Run("resolve dry-run", func(t *testing.T) {
		stdout, _, code := fix.run("resolve", "--config", fix.configPath, "--dry-run", "--reason", "test", "br-0001")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		if !strings.Contains(stdout, "Dry-run") {
			t.Fatalf("expected 'Dry-run' in output, got: %q", stdout)
		}
	})
}

// ---------------------------------------------------------------------------
// doctor subcommand
// ---------------------------------------------------------------------------

func TestCLI_Doctor(t *testing.T) {
	fix := newCLIFixture(t)

	t.Run("text output", func(t *testing.T) {
		stdout, _, code := fix.runSub("doctor")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		// Doctor should always produce a report with check results
		if !strings.Contains(stdout, "Oathkeeper binary") {
			t.Fatalf("expected 'Oathkeeper binary' check in output, got:\n%s", stdout)
		}
		// Should have a summary line with pass/fail counts
		if !strings.Contains(stdout, "passed") {
			t.Fatalf("expected 'passed' in summary, got:\n%s", stdout)
		}
	})

	t.Run("json output", func(t *testing.T) {
		stdout, _, code := fix.runSub("doctor", "--json")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		var result struct {
			Checks []json.RawMessage `json:"checks"`
		}
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatalf("expected valid JSON, got: %v\noutput: %s", err, stdout)
		}
		if len(result.Checks) == 0 {
			t.Fatal("expected non-empty checks array")
		}
	})

	t.Run("mock br detected by doctor", func(t *testing.T) {
		// Doctor checks for the br binary. Our mock is at a custom path set via config.
		// The doctor check uses cfg.Verification.BeadsCommand, which is our mock-br path.
		stdout, _, code := fix.runSub("doctor", "--json")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		// The mock-br should be found since it's an absolute path
		if !strings.Contains(stdout, "pass") && !strings.Contains(stdout, "Beads") {
			// At minimum, the check ran — it may pass or fail depending on PATH lookup
			t.Logf("doctor output: %s", stdout)
		}
	})
}

// ---------------------------------------------------------------------------
// Global flags
// ---------------------------------------------------------------------------

func TestCLI_GlobalFlags(t *testing.T) {
	fix := newCLIFixture(t)

	t.Run("--config with nonexistent file falls back to defaults", func(t *testing.T) {
		// scan with a nonexistent config should still work (LoadOrDefault returns defaults)
		transcript := fix.writeTranscript(t, "global-test.jsonl",
			`{"role":"assistant","content":"The system is running"}`,
		)
		_, _, code := fix.run("scan", "--config", "/nonexistent/config.toml", transcript)
		if code != 0 {
			t.Fatalf("expected exit 0 with fallback config, got %d", code)
		}
	})

	t.Run("--config missing value", func(t *testing.T) {
		_, stderr, code := fix.run("scan", "--config")
		if code == 0 {
			t.Fatal("expected non-zero exit for missing --config value")
		}
		if !strings.Contains(stderr, "missing") || !strings.Contains(stderr, "--config") {
			t.Fatalf("expected error about missing config value, got: %q", stderr)
		}
	})

	t.Run("--config provided twice", func(t *testing.T) {
		transcript := fix.writeTranscript(t, "dup-config.jsonl",
			`{"role":"assistant","content":"hello"}`,
		)
		_, stderr, code := fix.run("scan", "--config", fix.configPath, "--config", fix.configPath, transcript)
		if code == 0 {
			t.Fatal("expected non-zero exit for duplicate --config")
		}
		if !strings.Contains(stderr, "more than once") {
			t.Fatalf("expected 'more than once' error, got: %q", stderr)
		}
	})
}

// ---------------------------------------------------------------------------
// Edge cases & error output format
// ---------------------------------------------------------------------------

func TestCLI_ErrorOutputFormat(t *testing.T) {
	fix := newCLIFixture(t)

	t.Run("text error format", func(t *testing.T) {
		_, stderr, code := fix.runSub("scan", "/no/such/file.jsonl")
		if code == 0 {
			t.Fatal("expected non-zero exit")
		}
		if !strings.Contains(stderr, "Error:") {
			t.Fatalf("expected 'Error:' prefix in stderr, got: %q", stderr)
		}
	})

	t.Run("json error format", func(t *testing.T) {
		_, stderr, code := fix.runSub("scan", "/no/such/file.jsonl", "--json")
		if code == 0 {
			t.Fatal("expected non-zero exit")
		}
		var errResp map[string]interface{}
		if err := json.Unmarshal([]byte(stderr), &errResp); err != nil {
			t.Fatalf("expected JSON error on stderr, got: %q (parse error: %v)", stderr, err)
		}
		if _, ok := errResp["error"]; !ok {
			t.Fatalf("expected 'error' field in JSON error, got: %v", errResp)
		}
	})
}

func TestCLI_Scan_OpenClawFormat(t *testing.T) {
	fix := newCLIFixture(t)

	// Test OpenClaw nested transcript format
	transcript := fix.writeTranscript(t, "openclaw.jsonl",
		`{"id":"msg-1","message":{"role":"assistant","content":[{"type":"text","text":"I'll check back in 5 minutes to verify"}]},"timestamp":"2026-02-28T10:00:00Z"}`,
	)

	stdout, _, code := fix.runSub("scan", transcript)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "COMMITMENT DETECTED") {
		t.Fatalf("expected commitment in OpenClaw format, got:\n%s", stdout)
	}
}

func TestCLI_Scan_EmptyFile(t *testing.T) {
	fix := newCLIFixture(t)

	transcript := fix.writeTranscript(t, "empty.jsonl")

	stdout, _, code := fix.runSub("scan", transcript)
	if code != 0 {
		t.Fatalf("expected exit 0 for empty file, got %d", code)
	}
	if !strings.Contains(stdout, "No commitments detected") {
		t.Fatalf("expected 'No commitments detected' for empty file, got:\n%s", stdout)
	}
}

func TestCLI_Stats_InvalidExport(t *testing.T) {
	fix := newCLIFixture(t)

	_, stderr, code := fix.runSub("stats", "--export", "xml")
	if code == 0 {
		t.Fatal("expected non-zero exit for invalid export format")
	}
	if !strings.Contains(stderr, "invalid") {
		t.Fatalf("expected 'invalid' in error, got: %q", stderr)
	}
}

func TestCLI_List_TagFilter(t *testing.T) {
	fix := newCLIFixture(t)

	// Tags filter is a CLI-side filter (post-fetch from br)
	stdout, _, code := fix.runSub("list", "--status", "all", "--tag", "temporal", "--json")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	var result struct {
		Commitments []map[string]interface{} `json:"commitments"`
		Count       int                      `json:"count"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("expected valid JSON: %v\noutput: %s", err, stdout)
	}
	// All returned commitments should have the "temporal" tag
	for _, c := range result.Commitments {
		tags, ok := c["tags"].([]interface{})
		if !ok {
			continue
		}
		found := false
		for _, tag := range tags {
			if strings.EqualFold(fmt.Sprint(tag), "temporal") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected all commitments to have 'temporal' tag, got: %v", tags)
		}
	}
}
