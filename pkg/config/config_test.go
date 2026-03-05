package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.General.GracePeriod != 30 {
		t.Errorf("expected grace_period=30, got %d", cfg.General.GracePeriod)
	}
	if cfg.General.RecheckInterval != 300 {
		t.Errorf("expected recheck_interval=300, got %d", cfg.General.RecheckInterval)
	}
	if cfg.General.MaxAlerts != 3 {
		t.Errorf("expected max_alerts=3, got %d", cfg.General.MaxAlerts)
	}
	if cfg.General.Verbose {
		t.Error("expected verbose=false")
	}
	if cfg.General.ContextWindowSize != 5 {
		t.Errorf("expected context_window_size=5, got %d", cfg.General.ContextWindowSize)
	}
	if !cfg.General.MonitorTranscripts {
		t.Error("expected monitor_transcripts=true")
	}
	if cfg.General.TranscriptPollInterval != 3 {
		t.Errorf("expected transcript_poll_interval=3, got %d", cfg.General.TranscriptPollInterval)
	}
	if cfg.General.ReadinessErrorThreshold != 5 {
		t.Errorf("expected readiness_error_threshold=5, got %d", cfg.General.ReadinessErrorThreshold)
	}
	if cfg.General.ReadinessErrorWindow != 300 {
		t.Errorf("expected readiness_error_window=300, got %d", cfg.General.ReadinessErrorWindow)
	}
	if cfg.Server.Addr != ":9876" {
		t.Errorf("expected server addr=:9876, got %s", cfg.Server.Addr)
	}
	if cfg.OpenClaw.APIURL != "http://localhost:8080" {
		t.Errorf("unexpected openclaw api_url: %s", cfg.OpenClaw.APIURL)
	}
	if cfg.Alerts.OpenClawEnabled != true {
		t.Error("expected openclaw_enabled=true")
	}
	if cfg.Alerts.TelegramEnabled != false {
		t.Error("expected telegram_enabled=false")
	}
	if cfg.Alerts.ResolutionWebhook != "" {
		t.Errorf("expected resolution_webhook empty by default, got %q", cfg.Alerts.ResolutionWebhook)
	}
	if cfg.Alerts.ThrottleWindow != 3600 {
		t.Errorf("expected throttle_window=3600, got %d", cfg.Alerts.ThrottleWindow)
	}
	if cfg.Relay.Enabled {
		t.Error("expected relay.enabled=false")
	}
	if cfg.Relay.Command != "relay" {
		t.Errorf("expected relay.command=relay, got %q", cfg.Relay.Command)
	}
	if cfg.Relay.To != "athena" {
		t.Errorf("expected relay.to=athena, got %q", cfg.Relay.To)
	}
	if cfg.Relay.From != "oathkeeper" {
		t.Errorf("expected relay.from=oathkeeper, got %q", cfg.Relay.From)
	}
	if cfg.Relay.Timeout != 5 {
		t.Errorf("expected relay.timeout=5, got %d", cfg.Relay.Timeout)
	}
	if cfg.Relay.Retries != 2 {
		t.Errorf("expected relay.retries=2, got %d", cfg.Relay.Retries)
	}
	if cfg.Storage.AutoExpireHours != 168 {
		t.Errorf("expected auto_expire_hours=168, got %d", cfg.Storage.AutoExpireHours)
	}
	if cfg.Detector.MinConfidence != 0.7 {
		t.Errorf("expected min_confidence=0.7, got %f", cfg.Detector.MinConfidence)
	}
	if !cfg.Detector.PatternMatchingEnabled {
		t.Error("expected pattern_matching_enabled=true")
	}
	if len(cfg.Detector.Categories) != 4 {
		t.Errorf("expected 4 categories, got %d", len(cfg.Detector.Categories))
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "oathkeeper.toml")

	content := `
[general]
grace_period = 60
recheck_interval = 120
max_alerts = 5
verbose = true
monitor_transcripts = false
transcript_poll_interval = 9
readiness_error_threshold = 2
readiness_error_window = 60

[openclaw]
api_url = "http://example.com:9090"
transcript_dir = "/tmp/transcripts"

[alerts]
openclaw_enabled = false
telegram_enabled = true
telegram_webhook = "http://argus.local:9090/webhook/telegram"
resolution_webhook = "http://argus.local:9090/webhook/resolve"
throttle_window = 1800

[relay]
enabled = true
command = "relay-test"
to = "athena-test"
from = "oathkeeper-test"
timeout = 12
retries = 4

[storage]
db_path = "/tmp/test.db"
auto_expire_hours = 48

[detector]
min_confidence = 0.9
pattern_matching_enabled = false
categories = ["temporal", "conditional"]

[verification]
state_dirs = ["/tmp/state"]
memory_dirs = ["/tmp/memory"]
beads_command = "br-test"
tmux_command = "tmux-test"

[llm]
command = "claude-test"
args = ["-p", "--model", "sonnet"]
timeout = 20
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.General.GracePeriod != 60 {
		t.Errorf("expected grace_period=60, got %d", cfg.General.GracePeriod)
	}
	if cfg.General.RecheckInterval != 120 {
		t.Errorf("expected recheck_interval=120, got %d", cfg.General.RecheckInterval)
	}
	if cfg.General.MaxAlerts != 5 {
		t.Errorf("expected max_alerts=5, got %d", cfg.General.MaxAlerts)
	}
	if !cfg.General.Verbose {
		t.Error("expected verbose=true")
	}
	if cfg.General.MonitorTranscripts {
		t.Error("expected monitor_transcripts=false")
	}
	if cfg.General.TranscriptPollInterval != 9 {
		t.Errorf("expected transcript_poll_interval=9, got %d", cfg.General.TranscriptPollInterval)
	}
	if cfg.General.ReadinessErrorThreshold != 2 {
		t.Errorf("expected readiness_error_threshold=2, got %d", cfg.General.ReadinessErrorThreshold)
	}
	if cfg.General.ReadinessErrorWindow != 60 {
		t.Errorf("expected readiness_error_window=60, got %d", cfg.General.ReadinessErrorWindow)
	}
	if cfg.OpenClaw.APIURL != "http://example.com:9090" {
		t.Errorf("unexpected api_url: %s", cfg.OpenClaw.APIURL)
	}
	if cfg.OpenClaw.TranscriptDir != "/tmp/transcripts" {
		t.Errorf("unexpected transcript_dir: %s", cfg.OpenClaw.TranscriptDir)
	}
	if cfg.Alerts.OpenClawEnabled {
		t.Error("expected openclaw_enabled=false")
	}
	if !cfg.Alerts.TelegramEnabled {
		t.Error("expected telegram_enabled=true")
	}
	if cfg.Alerts.TelegramWebhook != "http://argus.local:9090/webhook/telegram" {
		t.Errorf("unexpected telegram_webhook: %s", cfg.Alerts.TelegramWebhook)
	}
	if cfg.Alerts.ResolutionWebhook != "http://argus.local:9090/webhook/resolve" {
		t.Errorf("unexpected resolution_webhook: %s", cfg.Alerts.ResolutionWebhook)
	}
	if cfg.Alerts.ThrottleWindow != 1800 {
		t.Errorf("expected throttle_window=1800, got %d", cfg.Alerts.ThrottleWindow)
	}
	if !cfg.Relay.Enabled {
		t.Error("expected relay.enabled=true")
	}
	if cfg.Relay.Command != "relay-test" {
		t.Errorf("unexpected relay command: %s", cfg.Relay.Command)
	}
	if cfg.Relay.To != "athena-test" {
		t.Errorf("unexpected relay to: %s", cfg.Relay.To)
	}
	if cfg.Relay.From != "oathkeeper-test" {
		t.Errorf("unexpected relay from: %s", cfg.Relay.From)
	}
	if cfg.Relay.Timeout != 12 {
		t.Errorf("expected relay timeout=12, got %d", cfg.Relay.Timeout)
	}
	if cfg.Relay.Retries != 4 {
		t.Errorf("expected relay retries=4, got %d", cfg.Relay.Retries)
	}
	if cfg.Storage.DBPath != "/tmp/test.db" {
		t.Errorf("unexpected db_path: %s", cfg.Storage.DBPath)
	}
	if cfg.Storage.AutoExpireHours != 48 {
		t.Errorf("expected auto_expire_hours=48, got %d", cfg.Storage.AutoExpireHours)
	}
	if cfg.Detector.MinConfidence != 0.9 {
		t.Errorf("expected min_confidence=0.9, got %f", cfg.Detector.MinConfidence)
	}
	if cfg.Detector.PatternMatchingEnabled {
		t.Error("expected pattern_matching_enabled=false")
	}
	if len(cfg.Detector.Categories) != 2 {
		t.Errorf("expected 2 categories, got %d", len(cfg.Detector.Categories))
	}
	if cfg.Verification.BeadsCommand != "br-test" {
		t.Errorf("unexpected beads_command: %s", cfg.Verification.BeadsCommand)
	}
	if cfg.Verification.TmuxCommand != "tmux-test" {
		t.Errorf("unexpected tmux_command: %s", cfg.Verification.TmuxCommand)
	}
	if cfg.LLM.Command != "claude-test" {
		t.Errorf("unexpected llm command: %s", cfg.LLM.Command)
	}
	if cfg.LLM.Timeout != 20 {
		t.Errorf("expected llm timeout=20, got %d", cfg.LLM.Timeout)
	}
}

func TestLLMConfigNoFallbackEnabledField(t *testing.T) {
	if _, ok := reflect.TypeOf(LLMConfig{}).FieldByName("FallbackEnabled"); ok {
		t.Fatal("LLMConfig should not expose FallbackEnabled")
	}
}

func TestLoadPartialConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "oathkeeper.toml")

	// Only override grace_period — everything else should get defaults
	content := `
[general]
grace_period = 10
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.General.GracePeriod != 10 {
		t.Errorf("expected grace_period=10, got %d", cfg.General.GracePeriod)
	}
	// Defaults should still be set for unspecified fields
	if cfg.General.MaxAlerts != 3 {
		t.Errorf("expected default max_alerts=3, got %d", cfg.General.MaxAlerts)
	}
	if !cfg.General.MonitorTranscripts {
		t.Error("expected default monitor_transcripts=true")
	}
	if cfg.General.TranscriptPollInterval != 3 {
		t.Errorf("expected default transcript_poll_interval=3, got %d", cfg.General.TranscriptPollInterval)
	}
	if cfg.General.ReadinessErrorThreshold != 5 {
		t.Errorf("expected default readiness_error_threshold=5, got %d", cfg.General.ReadinessErrorThreshold)
	}
	if cfg.General.ReadinessErrorWindow != 300 {
		t.Errorf("expected default readiness_error_window=300, got %d", cfg.General.ReadinessErrorWindow)
	}
	if cfg.OpenClaw.APIURL != "http://localhost:8080" {
		t.Errorf("expected default api_url, got %s", cfg.OpenClaw.APIURL)
	}
	if cfg.Alerts.ThrottleWindow != 3600 {
		t.Errorf("expected default throttle_window=3600, got %d", cfg.Alerts.ThrottleWindow)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/oathkeeper.toml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadInvalidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "oathkeeper.toml")

	if err := os.WriteFile(path, []byte("this is not valid TOML [[["), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestGracePeriodDuration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.General.GracePeriod = 45

	d := cfg.GracePeriodDuration()
	if d != 45*time.Second {
		t.Errorf("expected 45s, got %v", d)
	}
}

func TestRecheckIntervalDuration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.General.RecheckInterval = 120

	d := cfg.RecheckIntervalDuration()
	if d != 120*time.Second {
		t.Errorf("expected 120s (2m), got %v", d)
	}
}

func TestTranscriptPollIntervalDuration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.General.TranscriptPollInterval = 11

	d := cfg.TranscriptPollIntervalDuration()
	if d != 11*time.Second {
		t.Errorf("expected 11s, got %v", d)
	}
}

func TestReadinessErrorWindowDuration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.General.ReadinessErrorWindow = 45

	d := cfg.ReadinessErrorWindowDuration()
	if d != 45*time.Second {
		t.Errorf("expected 45s, got %v", d)
	}
}

func TestThrottleWindowDuration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Alerts.ThrottleWindow = 1800

	d := cfg.ThrottleWindowDuration()
	if d != 1800*time.Second {
		t.Errorf("expected 1800s (30m), got %v", d)
	}
}

func TestLLMTimeoutDuration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LLM.Timeout = 15

	d := cfg.LLMTimeoutDuration()
	if d != 15*time.Second {
		t.Errorf("expected 15s, got %v", d)
	}
}

func TestRelayTimeoutDuration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Relay.Timeout = 9

	d := cfg.RelayTimeoutDuration()
	if d != 9*time.Second {
		t.Errorf("expected 9s, got %v", d)
	}
}

func TestDefaultDBPath(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Storage.DBPath != "~/.local/share/oathkeeper/commitments.db" {
		t.Errorf("unexpected default db_path: %s", cfg.Storage.DBPath)
	}
}

func TestDefaultConfigPath(t *testing.T) {
	p := DefaultConfigPath()
	expected := "~/.config/oathkeeper/oathkeeper.toml"
	if p != expected {
		t.Errorf("expected %s, got %s", expected, p)
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get home dir")
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"~/.config/oathkeeper/oathkeeper.toml", filepath.Join(home, ".config/oathkeeper/oathkeeper.toml")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, tc := range tests {
		result := ExpandPath(tc.input)
		if result != tc.expected {
			t.Errorf("ExpandPath(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestLoadOrDefault_NoFile(t *testing.T) {
	cfg := LoadOrDefault("/nonexistent/oathkeeper.toml")
	if cfg.General.GracePeriod != 30 {
		t.Errorf("expected default grace_period=30, got %d", cfg.General.GracePeriod)
	}
}

func TestLoadContextWindowSizeAndServerAddr(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "oathkeeper.toml")

	content := `
[general]
context_window_size = 10

[server]
addr = ":8080"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.General.ContextWindowSize != 10 {
		t.Errorf("expected context_window_size=10, got %d", cfg.General.ContextWindowSize)
	}
	if cfg.Server.Addr != ":8080" {
		t.Errorf("expected server addr=:8080, got %s", cfg.Server.Addr)
	}
}

func TestLoadContextWindowSizeDefaultFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "oathkeeper.toml")

	// No context_window_size or server section — should get defaults
	content := `
[general]
grace_period = 15
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.General.ContextWindowSize != 5 {
		t.Errorf("expected default context_window_size=5, got %d", cfg.General.ContextWindowSize)
	}
	if cfg.Server.Addr != ":9876" {
		t.Errorf("expected default server addr=:9876, got %s", cfg.Server.Addr)
	}
}

func TestLoadOrDefault_WithFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "oathkeeper.toml")

	content := `
[general]
grace_period = 99
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := LoadOrDefault(path)
	if cfg.General.GracePeriod != 99 {
		t.Errorf("expected grace_period=99, got %d", cfg.General.GracePeriod)
	}
}
