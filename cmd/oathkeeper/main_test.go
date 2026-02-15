package main

import (
	"strings"
	"testing"
)

func TestUsageContainsAllSubcommands(t *testing.T) {
	commands := []string{"serve", "scan", "list", "stats", "resolve", "doctor"}
	for _, cmd := range commands {
		if !strings.Contains(usage, cmd) {
			t.Errorf("usage text missing subcommand %q", cmd)
		}
	}
}

func TestUsageContainsConfigFlag(t *testing.T) {
	if !strings.Contains(usage, "--config") {
		t.Error("usage text missing --config flag")
	}
}

func TestUsageContainsVersionFlag(t *testing.T) {
	if !strings.Contains(usage, "--version") {
		t.Error("usage text missing --version flag")
	}
}

func TestLoadConfigDefaultPath(t *testing.T) {
	cfg := loadConfig("")
	if cfg == nil {
		t.Fatal("loadConfig returned nil for default path")
	}
	if cfg.Verification.BeadsCommand != "br" {
		t.Fatalf("expected default beads command 'br', got %q", cfg.Verification.BeadsCommand)
	}
}

func TestLoadConfigNonexistentPath(t *testing.T) {
	cfg := loadConfig("/tmp/nonexistent-oathkeeper-config.toml")
	if cfg == nil {
		t.Fatal("loadConfig returned nil for nonexistent path")
	}
	// Should fall back to defaults
	if cfg.General.GracePeriod != 30 {
		t.Fatalf("expected default grace period 30, got %d", cfg.General.GracePeriod)
	}
}

func TestExtractConfigFlag(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantConfig string
		wantRest   []string
	}{
		{
			name:       "no config flag",
			args:       []string{"somefile.jsonl"},
			wantConfig: "",
			wantRest:   []string{"somefile.jsonl"},
		},
		{
			name:       "config before file",
			args:       []string{"--config", "/tmp/c.toml", "somefile.jsonl"},
			wantConfig: "/tmp/c.toml",
			wantRest:   []string{"somefile.jsonl"},
		},
		{
			name:       "config after file",
			args:       []string{"somefile.jsonl", "--config", "/tmp/c.toml"},
			wantConfig: "/tmp/c.toml",
			wantRest:   []string{"somefile.jsonl"},
		},
		{
			name:       "empty args",
			args:       []string{},
			wantConfig: "",
			wantRest:   nil,
		},
		{
			name:       "config without value",
			args:       []string{"--config"},
			wantConfig: "",
			wantRest:   []string{"--config"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotConfig, gotRest := extractConfigFlag(tt.args)
			if gotConfig != tt.wantConfig {
				t.Errorf("config = %q, want %q", gotConfig, tt.wantConfig)
			}
			if len(gotRest) != len(tt.wantRest) {
				t.Errorf("rest = %v, want %v", gotRest, tt.wantRest)
				return
			}
			for i, v := range gotRest {
				if v != tt.wantRest[i] {
					t.Errorf("rest[%d] = %q, want %q", i, v, tt.wantRest[i])
				}
			}
		})
	}
}

func TestVersionConstDefined(t *testing.T) {
	if version == "" {
		t.Error("version constant is empty")
	}
}
