package config

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the top-level configuration for Oathkeeper.
type Config struct {
	General      GeneralConfig      `toml:"general"`
	Server       ServerConfig       `toml:"server"`
	OpenClaw     OpenClawConfig     `toml:"openclaw"`
	LLM          LLMConfig          `toml:"llm"`
	Verification VerificationConfig `toml:"verification"`
	Alerts       AlertsConfig       `toml:"alerts"`
	Storage      StorageConfig      `toml:"storage"`
	Detector     DetectorConfig     `toml:"detector"`
}

// GeneralConfig holds top-level operational settings.
type GeneralConfig struct {
	GracePeriod       int  `toml:"grace_period"`
	RecheckInterval   int  `toml:"recheck_interval"`
	MaxAlerts         int  `toml:"max_alerts"`
	Verbose           bool `toml:"verbose"`
	ContextWindowSize int  `toml:"context_window_size"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Addr string `toml:"addr"`
}

// OpenClawConfig holds OpenClaw integration settings.
type OpenClawConfig struct {
	APIURL        string `toml:"api_url"`
	TranscriptDir string `toml:"transcript_dir"`
	WakeEndpoint  string `toml:"wake_endpoint"`
	CronEndpoint  string `toml:"cron_endpoint"`
}

// LLMConfig holds LLM classification settings.
type LLMConfig struct {
	Command string   `toml:"command"`
	Args    []string `toml:"args"`
	Timeout int      `toml:"timeout"`
}

// VerificationConfig holds mechanism verification settings.
type VerificationConfig struct {
	StateDirs    []string `toml:"state_dirs"`
	MemoryDirs   []string `toml:"memory_dirs"`
	BeadsCommand string   `toml:"beads_command"`
	TmuxCommand  string   `toml:"tmux_command"`
}

// AlertsConfig holds alert destination settings.
type AlertsConfig struct {
	OpenClawEnabled  bool   `toml:"openclaw_enabled"`
	TelegramEnabled  bool   `toml:"telegram_enabled"`
	TelegramWebhook  string `toml:"telegram_webhook"`
	ThrottleWindow   int    `toml:"throttle_window"`
}

// StorageConfig holds database settings.
type StorageConfig struct {
	DBPath          string `toml:"db_path"`
	AutoExpireHours int    `toml:"auto_expire_hours"`
}

// DetectorConfig holds detection sensitivity settings.
type DetectorConfig struct {
	MinConfidence          float64  `toml:"min_confidence"`
	PatternMatchingEnabled bool     `toml:"pattern_matching_enabled"`
	Categories             []string `toml:"categories"`
}

// DefaultConfig returns a Config with all default values matching the PRD spec.
func DefaultConfig() *Config {
	return &Config{
		General: GeneralConfig{
			GracePeriod:       30,
			RecheckInterval:   300,
			MaxAlerts:         3,
			Verbose:           false,
			ContextWindowSize: 5,
		},
		Server: ServerConfig{
			Addr: ":9876",
		},
		OpenClaw: OpenClawConfig{
			APIURL:        "http://localhost:8080",
			TranscriptDir: "~/.openclaw/sessions",
			WakeEndpoint:  "/api/v1/sessions/{session}/wake",
			CronEndpoint:  "/api/v1/crons",
		},
		LLM: LLMConfig{
			Command: "claude",
			Args:    []string{"-p", "--model", "haiku"},
			Timeout: 10,
		},
		Verification: VerificationConfig{
			StateDirs:    []string{"~/.openclaw/state"},
			MemoryDirs:   []string{"~/.openclaw/memory"},
			BeadsCommand: "br",
			TmuxCommand:  "tmux",
		},
		Alerts: AlertsConfig{
			OpenClawEnabled:  true,
			TelegramEnabled:  false,
			TelegramWebhook:  "http://localhost:9090/webhook/telegram",
			ThrottleWindow:   3600,
		},
		Storage: StorageConfig{
			DBPath:          "~/.local/share/oathkeeper/commitments.db",
			AutoExpireHours: 168,
		},
		Detector: DetectorConfig{
			MinConfidence:          0.7,
			PatternMatchingEnabled: true,
			Categories:             []string{"temporal", "scheduled", "followup", "conditional"},
		},
	}
}

// DefaultConfigPath returns the default config file path.
func DefaultConfigPath() string {
	return "~/.config/oathkeeper/oathkeeper.toml"
}

// ExpandPath expands a leading ~ to the user's home directory.
func ExpandPath(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}

// Load reads and parses a TOML config file, applying defaults for unset fields.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// LoadOrDefault tries to load a config file; returns defaults if the file doesn't exist.
func LoadOrDefault(path string) *Config {
	cfg, err := Load(path)
	if err != nil {
		return DefaultConfig()
	}
	return cfg
}

// GracePeriodDuration returns the grace period as a time.Duration.
func (c *Config) GracePeriodDuration() time.Duration {
	return time.Duration(c.General.GracePeriod) * time.Second
}

// RecheckIntervalDuration returns the recheck interval as a time.Duration.
func (c *Config) RecheckIntervalDuration() time.Duration {
	return time.Duration(c.General.RecheckInterval) * time.Second
}

// ThrottleWindowDuration returns the throttle window as a time.Duration.
func (c *Config) ThrottleWindowDuration() time.Duration {
	return time.Duration(c.Alerts.ThrottleWindow) * time.Second
}

// LLMTimeoutDuration returns the LLM timeout as a time.Duration.
func (c *Config) LLMTimeoutDuration() time.Duration {
	return time.Duration(c.LLM.Timeout) * time.Second
}
