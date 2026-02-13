package doctor

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckResult(t *testing.T) {
	t.Run("pass result", func(t *testing.T) {
		r := CheckResult{
			Name:     "SQLite database",
			Status:   StatusPass,
			Detail:   "~/.local/share/oathkeeper/commitments.db (accessible)",
			Required: true,
		}
		if r.Status != StatusPass {
			t.Errorf("expected pass, got %s", r.Status)
		}
	})

	t.Run("fail result", func(t *testing.T) {
		r := CheckResult{
			Name:     "OpenClaw API",
			Status:   StatusFail,
			Detail:   "http://localhost:8080 (unreachable)",
			Required: true,
		}
		if r.Status != StatusFail {
			t.Errorf("expected fail, got %s", r.Status)
		}
	})

	t.Run("warn result for optional dependency", func(t *testing.T) {
		r := CheckResult{
			Name:     "Argus bot",
			Status:   StatusWarn,
			Detail:   "not configured (optional)",
			Required: false,
		}
		if r.Status != StatusWarn {
			t.Errorf("expected warn, got %s", r.Status)
		}
	})
}

func TestFormatResult(t *testing.T) {
	t.Run("pass uses checkmark", func(t *testing.T) {
		r := CheckResult{Name: "Tmux", Status: StatusPass, Detail: "version 3.3a (found)"}
		out := FormatResult(r)
		expected := "[✓] Tmux: version 3.3a (found)"
		if out != expected {
			t.Errorf("expected %q, got %q", expected, out)
		}
	})

	t.Run("fail uses cross", func(t *testing.T) {
		r := CheckResult{Name: "OpenClaw API", Status: StatusFail, Detail: "unreachable"}
		out := FormatResult(r)
		expected := "[✗] OpenClaw API: unreachable"
		if out != expected {
			t.Errorf("expected %q, got %q", expected, out)
		}
	})

	t.Run("warn uses exclamation", func(t *testing.T) {
		r := CheckResult{Name: "Argus bot", Status: StatusWarn, Detail: "not configured (optional)"}
		out := FormatResult(r)
		expected := "[!] Argus bot: not configured (optional)"
		if out != expected {
			t.Errorf("expected %q, got %q", expected, out)
		}
	})
}

func TestCheckBinary(t *testing.T) {
	t.Run("finds existing binary", func(t *testing.T) {
		// "go" should be findable via exec.LookPath
		r := CheckBinary("Go compiler", "go", "--version")
		if r.Status != StatusPass {
			t.Errorf("expected pass for 'go', got %s: %s", r.Status, r.Detail)
		}
		if r.Name != "Go compiler" {
			t.Errorf("expected name 'Go compiler', got %q", r.Name)
		}
	})

	t.Run("fails for missing binary", func(t *testing.T) {
		r := CheckBinary("Missing tool", "nonexistent-binary-xyz123", "--version")
		if r.Status != StatusFail {
			t.Errorf("expected fail for missing binary, got %s", r.Status)
		}
	})
}

func TestCheckHTTPEndpoint(t *testing.T) {
	t.Run("reachable server", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		r := CheckHTTPEndpoint("Test API", srv.URL)
		if r.Status != StatusPass {
			t.Errorf("expected pass, got %s: %s", r.Status, r.Detail)
		}
	})

	t.Run("unreachable server", func(t *testing.T) {
		r := CheckHTTPEndpoint("Bad API", "http://127.0.0.1:1")
		if r.Status != StatusFail {
			t.Errorf("expected fail for unreachable, got %s", r.Status)
		}
	})

	t.Run("server returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		r := CheckHTTPEndpoint("Error API", srv.URL)
		if r.Status != StatusFail {
			t.Errorf("expected fail for 500, got %s", r.Status)
		}
	})
}

func TestCheckFileAccessible(t *testing.T) {
	t.Run("existing file", func(t *testing.T) {
		// go.mod should exist in project root
		r := CheckFileAccessible("Config file", "../../go.mod")
		if r.Status != StatusPass {
			t.Errorf("expected pass for go.mod, got %s: %s", r.Status, r.Detail)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		r := CheckFileAccessible("Config file", "/nonexistent/path/to/file.toml")
		if r.Status != StatusFail {
			t.Errorf("expected fail for missing file, got %s", r.Status)
		}
	})
}

func TestFormatReport(t *testing.T) {
	results := []CheckResult{
		{Name: "Binary A", Status: StatusPass, Detail: "v1.0 (found)", Required: true},
		{Name: "Binary B", Status: StatusFail, Detail: "not found", Required: true},
		{Name: "Optional C", Status: StatusWarn, Detail: "not configured (optional)", Required: false},
	}

	report := FormatReport(results)

	// Should contain each formatted result
	if !containsStr(report, "[✓] Binary A: v1.0 (found)") {
		t.Error("missing pass line in report")
	}
	if !containsStr(report, "[✗] Binary B: not found") {
		t.Error("missing fail line in report")
	}
	if !containsStr(report, "[!] Optional C: not configured (optional)") {
		t.Error("missing warn line in report")
	}

	// Should contain summary
	if !containsStr(report, "1 passed") {
		t.Errorf("missing pass count in report: %s", report)
	}
	if !containsStr(report, "1 failed") {
		t.Errorf("missing fail count in report: %s", report)
	}
	if !containsStr(report, "1 warning") {
		t.Errorf("missing warn count in report: %s", report)
	}
}

func TestFormatReportAllPass(t *testing.T) {
	results := []CheckResult{
		{Name: "A", Status: StatusPass, Detail: "ok", Required: true},
		{Name: "B", Status: StatusPass, Detail: "ok", Required: true},
	}

	report := FormatReport(results)
	if !containsStr(report, "All required dependencies OK.") {
		t.Errorf("expected success message, got: %s", report)
	}
}

func TestFormatReportWithFailures(t *testing.T) {
	results := []CheckResult{
		{Name: "A", Status: StatusPass, Detail: "ok", Required: true},
		{Name: "B", Status: StatusFail, Detail: "missing", Required: true},
	}

	report := FormatReport(results)
	if containsStr(report, "All required dependencies OK.") {
		t.Error("should not show success message when there are failures")
	}
}

func TestRunChecks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := Config{
		Version:      "1.0.0",
		DBPath:       "../../go.mod", // use existing file as stand-in
		ConfigPath:   "/nonexistent/config.toml",
		OpenClawURL:  srv.URL,
		BeadsCommand: "nonexistent-br-xyz",
		TmuxCommand:  "nonexistent-tmux-xyz",
		ClaudeCommand: "nonexistent-claude-xyz",
		ArgusWebhook: "",
	}

	results := RunChecks(cfg)

	// Should have 7 checks (version, db, config, openclaw, beads, tmux, claude, argus)
	if len(results) < 7 {
		t.Errorf("expected at least 7 check results, got %d", len(results))
		for _, r := range results {
			t.Logf("  %s: %s - %s", r.Name, r.Status, r.Detail)
		}
	}

	// Version check should pass
	if results[0].Status != StatusPass {
		t.Errorf("version check should pass, got %s", results[0].Status)
	}

	// DB path (go.mod exists) should pass
	if results[1].Status != StatusPass {
		t.Errorf("db check should pass, got %s: %s", results[1].Status, results[1].Detail)
	}

	// Config file (nonexistent) should fail
	if results[2].Status != StatusFail {
		t.Errorf("config check should fail for nonexistent, got %s", results[2].Status)
	}

	// OpenClaw should pass (test server)
	if results[3].Status != StatusPass {
		t.Errorf("openclaw check should pass, got %s: %s", results[3].Status, results[3].Detail)
	}

	// Find argus check (last one)
	argus := results[len(results)-1]
	if argus.Name != "Argus bot" {
		t.Errorf("expected last check to be Argus, got %q", argus.Name)
	}
	if argus.Status != StatusWarn {
		t.Errorf("argus with empty webhook should warn, got %s", argus.Status)
	}
}

func TestArgusCheckConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := Config{
		ArgusWebhook: srv.URL,
	}

	r := checkArgus(cfg)
	if r.Status != StatusPass {
		t.Errorf("expected pass for reachable argus, got %s: %s", r.Status, r.Detail)
	}
}

func TestArgusCheckUnreachable(t *testing.T) {
	cfg := Config{
		ArgusWebhook: "http://127.0.0.1:1",
	}

	r := checkArgus(cfg)
	if r.Status != StatusFail {
		t.Errorf("expected fail for unreachable argus, got %s", r.Status)
	}
}

func TestVersionCheck(t *testing.T) {
	r := checkVersion("2.3.1")
	if r.Status != StatusPass {
		t.Errorf("expected pass, got %s", r.Status)
	}
	if r.Detail != "v2.3.1" {
		t.Errorf("expected detail 'v2.3.1', got %q", r.Detail)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && fmt.Sprintf("%s", s) != "" && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
