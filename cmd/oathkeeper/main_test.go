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
