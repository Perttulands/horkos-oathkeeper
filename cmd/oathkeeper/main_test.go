package main

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/perttulands/oathkeeper/pkg/beads"
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
	if !strings.Contains(usage, "--dry-run") {
		t.Error("usage text missing --dry-run flag")
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
	if cfg.Verification.BeadsCommand != "bd" {
		t.Fatalf("expected default beads command 'bd', got %q", cfg.Verification.BeadsCommand)
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
		wantErr    string
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
			name:    "config without value",
			args:    []string{"--config"},
			wantErr: "missing value for --config",
		},
		{
			name:       "config equals syntax",
			args:       []string{"--config=/tmp/c.toml", "somefile.jsonl"},
			wantConfig: "/tmp/c.toml",
			wantRest:   []string{"somefile.jsonl"},
		},
		{
			name:    "duplicate config",
			args:    []string{"--config", "/tmp/a.toml", "--config=/tmp/b.toml"},
			wantErr: "--config provided more than once",
		},
		{
			name:    "empty config value",
			args:    []string{"--config="},
			wantErr: "--config cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotConfig, gotRest, err := extractConfigFlag(tt.args)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotConfig != tt.wantConfig {
				t.Errorf("config = %q, want %q", gotConfig, tt.wantConfig)
			}
			if !reflect.DeepEqual(gotRest, tt.wantRest) {
				t.Errorf("rest = %v, want %v", gotRest, tt.wantRest)
			}
		})
	}
}

func TestExtractGlobalFlags(t *testing.T) {
	configPath, dryRun, rest, err := extractGlobalFlags([]string{"--dry-run", "--config", "/tmp/c.toml", "serve", "--tag", "ops"})
	if err != nil {
		t.Fatalf("extractGlobalFlags unexpected error: %v", err)
	}
	if configPath != "/tmp/c.toml" {
		t.Fatalf("configPath = %q, want /tmp/c.toml", configPath)
	}
	if !dryRun {
		t.Fatal("dryRun should be true")
	}
	want := []string{"serve", "--tag", "ops"}
	if !reflect.DeepEqual(rest, want) {
		t.Fatalf("rest = %v, want %v", rest, want)
	}
}

func TestParseTagValues(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []string
		wantErr string
	}{
		{name: "empty", raw: "", want: nil},
		{name: "single", raw: "ops", want: []string{"ops"}},
		{name: "comma separated trimmed", raw: "ops, temporal,ops", want: []string{"ops", "temporal"}},
		{name: "empty segment", raw: "ops,,temporal", wantErr: "tags must be comma-separated without empty values"},
		{name: "invalid chars", raw: "ops,team blue", wantErr: `"team blue" is not a valid tag (allowed: letters, numbers, '-', '_')`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTagValues(tt.raw)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("tags = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseScanArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    scanOptions
		wantErr string
	}{
		{
			name: "default text",
			args: []string{"transcript.jsonl"},
			want: scanOptions{file: "transcript.jsonl", format: "text", json: false},
		},
		{
			name: "json flag",
			args: []string{"--json", "transcript.jsonl"},
			want: scanOptions{file: "transcript.jsonl", format: "json", json: true},
		},
		{
			name:    "invalid format",
			args:    []string{"--format", "yaml", "transcript.jsonl"},
			wantErr: `invalid --format "yaml" (allowed: text, json)`,
		},
		{
			name:    "missing file",
			args:    []string{},
			wantErr: scanUsage,
		},
		{
			name:    "too many args",
			args:    []string{"a.jsonl", "b.jsonl"},
			wantErr: scanUsage,
		},
		{
			name:    "unknown flag",
			args:    []string{"--bogus", "a.jsonl"},
			wantErr: "flag provided but not defined: -bogus (run with --help for details)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseScanArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("options = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseServeArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    []string
		wantErr string
	}{
		{name: "default", args: nil, want: nil},
		{name: "tag list", args: []string{"--tag", "ops,incident"}, want: []string{"ops", "incident"}},
		{name: "invalid tag", args: []string{"--tag", "ops team"}, wantErr: `invalid --tag value: "ops team" is not a valid tag (allowed: letters, numbers, '-', '_')`},
		{name: "unexpected arg", args: []string{"extra"}, wantErr: "unexpected argument(s) for serve: extra"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseServeArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got.extraTags, tt.want) {
				t.Fatalf("extraTags = %v, want %v", got.extraTags, tt.want)
			}
		})
	}
}

func TestParseListArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		check   func(t *testing.T, got listOptions)
		wantErr string
	}{
		{
			name: "defaults",
			args: nil,
			check: func(t *testing.T, got listOptions) {
				t.Helper()
				if got.status != "open" || got.category != "" || got.since != 0 || got.json {
					t.Fatalf("unexpected defaults: %+v", got)
				}
			},
		},
		{
			name: "all flags",
			args: []string{"--status", "all", "--category", "temporal", "--since", "24h", "--tag", "ops,incident", "--json"},
			check: func(t *testing.T, got listOptions) {
				t.Helper()
				if got.status != "all" {
					t.Fatalf("status = %q, want all", got.status)
				}
				if got.category != "temporal" {
					t.Fatalf("category = %q, want temporal", got.category)
				}
				if got.since != 24*time.Hour {
					t.Fatalf("since = %v, want 24h", got.since)
				}
				if !got.json {
					t.Fatalf("json = false, want true")
				}
				if !reflect.DeepEqual(got.tags, []string{"ops", "incident"}) {
					t.Fatalf("tags = %v, want [ops incident]", got.tags)
				}
			},
		},
		{
			name:    "invalid status",
			args:    []string{"--status", "pending"},
			wantErr: `invalid --status "pending" (allowed: open, closed, all)`,
		},
		{
			name:    "invalid category",
			args:    []string{"--category", "team blue"},
			wantErr: `invalid --category "team blue"`,
		},
		{
			name:    "invalid since parse",
			args:    []string{"--since", "yesterday"},
			wantErr: `invalid --since value "yesterday" (example: 24h)`,
		},
		{
			name:    "invalid since non positive",
			args:    []string{"--since", "-1h"},
			wantErr: "--since must be greater than 0",
		},
		{
			name:    "invalid tag list",
			args:    []string{"--tag", "ops,,incident"},
			wantErr: "invalid --tag value: tags must be comma-separated without empty values",
		},
		{
			name:    "unexpected positional",
			args:    []string{"extra"},
			wantErr: "unexpected argument(s) for list: extra",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseListArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.check(t, got)
		})
	}
}

func TestParseResolveArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    resolveOptions
		wantErr string
	}{
		{
			name: "default reason",
			args: []string{"bd-123"},
			want: resolveOptions{beadID: "bd-123", reason: "resolved via CLI", json: false},
		},
		{
			name: "positional reason",
			args: []string{"bd-123", "manual verification"},
			want: resolveOptions{beadID: "bd-123", reason: "manual verification", json: false},
		},
		{
			name: "reason flag",
			args: []string{"--reason", "closed by webhook", "--json", "bd-123"},
			want: resolveOptions{beadID: "bd-123", reason: "closed by webhook", json: true},
		},
		{
			name:    "reason conflict",
			args:    []string{"--reason", "a", "bd-123", "b"},
			wantErr: "use either positional reason or --reason, not both",
		},
		{
			name:    "missing bead id",
			args:    nil,
			wantErr: resolveUsage,
		},
		{
			name:    "too many args",
			args:    []string{"bd-123", "a", "b"},
			wantErr: "too many arguments for resolve",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseResolveArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("options = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseDoctorAndStatsArgs(t *testing.T) {
	stats, err := parseStatsArgs([]string{"--json"})
	if err != nil {
		t.Fatalf("parseStatsArgs unexpected error: %v", err)
	}
	if !stats.json {
		t.Fatal("stats json should be true")
	}
	if stats.export != "" {
		t.Fatalf("expected empty export by default, got %q", stats.export)
	}

	statsExport, err := parseStatsArgs([]string{"--export", "csv", "--output", "/tmp/stats.csv"})
	if err != nil {
		t.Fatalf("parseStatsArgs export unexpected error: %v", err)
	}
	if statsExport.export != "csv" {
		t.Fatalf("expected export csv, got %q", statsExport.export)
	}
	if statsExport.output != "/tmp/stats.csv" {
		t.Fatalf("expected output path, got %q", statsExport.output)
	}

	if _, err := parseStatsArgs([]string{"--output", "/tmp/x"}); err == nil {
		t.Fatal("expected --output without --export to fail")
	}
	if _, err := parseStatsArgs([]string{"--export", "xml"}); err == nil {
		t.Fatal("expected invalid --export to fail")
	}

	doctor, err := parseDoctorArgs([]string{"--json"})
	if err != nil {
		t.Fatalf("parseDoctorArgs unexpected error: %v", err)
	}
	if !doctor.json {
		t.Fatal("doctor json should be true")
	}

	if _, err := parseDoctorArgs([]string{"extra"}); err == nil {
		t.Fatal("expected parseDoctorArgs to reject unexpected args")
	}
}

func TestWantsJSON(t *testing.T) {
	if !wantsJSON([]string{"--json", "x"}) {
		t.Fatal("expected wantsJSON true")
	}
	if wantsJSON([]string{"--format", "json"}) {
		t.Fatal("expected wantsJSON false when --json is absent")
	}
}

func TestVersionConstDefined(t *testing.T) {
	if version == "" {
		t.Error("version constant is empty")
	}
}

func TestBuildStatsSummaryExpandedFields(t *testing.T) {
	now := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
	list := []beads.Bead{
		{ID: "bd-1", Status: "open", Tags: []string{"oathkeeper", "temporal"}, CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "bd-2", Status: "closed", Tags: []string{"oathkeeper", "conditional"}, CreatedAt: now.Add(-48 * time.Hour)},
		{ID: "bd-3", Status: "backed", Tags: []string{"oathkeeper", "followup"}, CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "bd-4", Status: "alerted", Tags: []string{"oathkeeper", "temporal"}, CreatedAt: now.Add(-30 * time.Minute)},
		{ID: "bd-5", Status: "expired", Tags: []string{"oathkeeper"}, CreatedAt: now.Add(-10 * time.Minute)},
	}

	summary := buildStatsSummary(list, now)
	if summary.Total != 5 {
		t.Fatalf("total = %d, want 5", summary.Total)
	}
	if summary.Open != 1 || summary.Resolved != 1 || summary.Backed != 1 || summary.Alerted != 1 || summary.Expired != 1 {
		t.Fatalf("unexpected status counters: %+v", summary)
	}
	if summary.Recent24h != 4 {
		t.Fatalf("recent_24h = %d, want 4", summary.Recent24h)
	}
	if summary.OldestOpenAgeSeconds != int64((2 * time.Hour).Seconds()) {
		t.Fatalf("oldest_open_age_seconds = %d, want %d", summary.OldestOpenAgeSeconds, int64((2 * time.Hour).Seconds()))
	}
	if summary.ByCategory["temporal"] != 2 || summary.ByCategory["conditional"] != 1 || summary.ByCategory["followup"] != 1 {
		t.Fatalf("unexpected by_category: %v", summary.ByCategory)
	}
	if summary.ByStatus["open"] != 1 || summary.ByStatus["closed"] != 1 || summary.ByStatus["backed"] != 1 || summary.ByStatus["alerted"] != 1 || summary.ByStatus["expired"] != 1 {
		t.Fatalf("unexpected by_status: %v", summary.ByStatus)
	}
}

func TestRenderStatsCSV(t *testing.T) {
	csv := renderStatsCSV(statsSummary{
		Total:      2,
		Open:       1,
		Resolved:   1,
		ByStatus:   map[string]int{"open": 1, "closed": 1},
		ByCategory: map[string]int{"temporal": 2},
	})
	if !strings.Contains(csv, "metric,value\n") {
		t.Fatalf("missing csv header: %q", csv)
	}
	if !strings.Contains(csv, "total,2\n") {
		t.Fatalf("missing total row: %q", csv)
	}
	if !strings.Contains(csv, "status_open,1\n") || !strings.Contains(csv, "status_closed,1\n") {
		t.Fatalf("missing status rows: %q", csv)
	}
	if !strings.Contains(csv, "category_temporal,2\n") {
		t.Fatalf("missing category row: %q", csv)
	}
}
