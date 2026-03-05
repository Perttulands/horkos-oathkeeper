package main

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/perttulands/horkos-oathkeeper/pkg/beads"
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
			wantErr: "parse scan flags: flag provided but not defined: -bogus (run with --help for details)",
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
			args: []string{"br-123"},
			want: resolveOptions{beadID: "br-123", reason: "resolved via CLI", json: false},
		},
		{
			name: "positional reason",
			args: []string{"br-123", "manual verification"},
			want: resolveOptions{beadID: "br-123", reason: "manual verification", json: false},
		},
		{
			name: "reason flag",
			args: []string{"--reason", "closed by webhook", "--json", "br-123"},
			want: resolveOptions{beadID: "br-123", reason: "closed by webhook", json: true},
		},
		{
			name:    "reason conflict",
			args:    []string{"--reason", "a", "br-123", "b"},
			wantErr: "use either positional reason or --reason, not both",
		},
		{
			name:    "missing bead id",
			args:    nil,
			wantErr: resolveUsage,
		},
		{
			name:    "too many args",
			args:    []string{"br-123", "a", "b"},
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
	statsDashboard, err := parseStatsArgs([]string{"--dashboard", "/tmp/dashboard.html"})
	if err != nil {
		t.Fatalf("parseStatsArgs dashboard unexpected error: %v", err)
	}
	if statsDashboard.dashboard != "/tmp/dashboard.html" {
		t.Fatalf("expected dashboard path, got %q", statsDashboard.dashboard)
	}
	if _, err := parseStatsArgs([]string{"--dashboard", "/tmp/d.html", "--export", "csv"}); err == nil {
		t.Fatal("expected --dashboard + --export to fail")
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
		{ID: "br-1", Status: "open", Tags: []string{"oathkeeper", "temporal"}, CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "br-2", Status: "closed", Tags: []string{"oathkeeper", "conditional"}, CreatedAt: now.Add(-48 * time.Hour)},
		{ID: "br-3", Status: "backed", Tags: []string{"oathkeeper", "followup"}, CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "br-4", Status: "alerted", Tags: []string{"oathkeeper", "temporal"}, CreatedAt: now.Add(-30 * time.Minute)},
		{ID: "br-5", Status: "expired", Tags: []string{"oathkeeper"}, CreatedAt: now.Add(-10 * time.Minute)},
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

func TestRenderStatsDashboard(t *testing.T) {
	page := renderStatsDashboard(statsSummary{
		Total:      3,
		Open:       1,
		Resolved:   2,
		ByStatus:   map[string]int{"open": 1, "closed": 2},
		ByCategory: map[string]int{"temporal": 2},
	}, time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC))

	if !strings.Contains(page, "Oathkeeper Stats Dashboard") {
		t.Fatalf("missing dashboard title: %q", page)
	}
	if !strings.Contains(page, "<td>open</td><td>1</td>") {
		t.Fatalf("missing status row: %q", page)
	}
	if !strings.Contains(page, "<td>temporal</td><td>2</td>") {
		t.Fatalf("missing category row: %q", page)
	}
}

func TestRenderStatsConsoleDashboard(t *testing.T) {
	out := renderStatsConsoleDashboard(statsSummary{
		Total:                10,
		Open:                 4,
		Resolved:             3,
		Backed:               2,
		Alerted:              1,
		Expired:              0,
		Recent24h:            5,
		OldestOpenAgeSeconds: 7200,
		ByStatus:             map[string]int{"open": 4, "closed": 3},
		ByCategory:           map[string]int{"temporal": 6, "followup": 2},
	})

	if !strings.Contains(out, "Commitment Dashboard") {
		t.Fatalf("missing dashboard heading: %q", out)
	}
	if !strings.Contains(out, "Open") || !strings.Contains(out, "Resolved") {
		t.Fatalf("missing primary rows: %q", out)
	}
	if !strings.Contains(out, "By status:") || !strings.Contains(out, "By category:") {
		t.Fatalf("missing breakdown sections: %q", out)
	}
	if !strings.Contains(out, "40.0%") {
		t.Fatalf("missing expected percentage formatting: %q", out)
	}
	if !strings.Contains(out, "####") {
		t.Fatalf("missing expected bar rendering: %q", out)
	}
}

func TestRenderStatsConsoleDashboardZeroTotal(t *testing.T) {
	out := renderStatsConsoleDashboard(statsSummary{
		Total:      0,
		ByStatus:   map[string]int{},
		ByCategory: map[string]int{},
	})

	if !strings.Contains(out, "Commitment Dashboard") {
		t.Fatalf("missing dashboard heading for zero-total: %q", out)
	}
	// Zero total should render bars as all dashes and percentages as 0.0%
	if !strings.Contains(out, "0.0%") {
		t.Fatalf("expected 0.0%% for zero-total, got: %q", out)
	}
	if !strings.Contains(out, "--------------------") {
		t.Fatalf("expected all-dash bar for zero-total, got: %q", out)
	}
}

func TestFilterByTags(t *testing.T) {
	input := []beads.Bead{
		{ID: "br-1", Tags: []string{"oathkeeper", "temporal", "session-main"}},
		{ID: "br-2", Tags: []string{"oathkeeper", "followup", "session-other"}},
		{ID: "br-3", Tags: []string{"oathkeeper", "temporal", "session-other"}},
		{ID: "br-4", Tags: []string{"oathkeeper"}},
	}

	// Filter by single tag
	filtered := filterByTags(input, []string{"temporal"})
	if len(filtered) != 2 {
		t.Fatalf("expected 2 beads with temporal tag, got %d", len(filtered))
	}
	for _, b := range filtered {
		found := false
		for _, tag := range b.Tags {
			if strings.EqualFold(tag, "temporal") {
				found = true
			}
		}
		if !found {
			t.Fatalf("bead %s missing temporal tag", b.ID)
		}
	}

	// Filter by multiple tags (AND logic)
	filtered = filterByTags(input, []string{"temporal", "session-main"})
	if len(filtered) != 1 || filtered[0].ID != "br-1" {
		t.Fatalf("expected only br-1 for temporal+session-main, got %v", filtered)
	}

	// Filter with no matching tags
	filtered = filterByTags(input, []string{"nonexistent"})
	if len(filtered) != 0 {
		t.Fatalf("expected 0 beads for nonexistent tag, got %d", len(filtered))
	}

	// Empty tags filter returns all
	filtered = filterByTags(input, nil)
	if len(filtered) != 4 {
		t.Fatalf("expected all 4 beads with nil tags, got %d", len(filtered))
	}
	filtered = filterByTags(input, []string{})
	if len(filtered) != 4 {
		t.Fatalf("expected all 4 beads with empty tags, got %d", len(filtered))
	}
}

func TestHasAllTags(t *testing.T) {
	beadTags := []string{"oathkeeper", "temporal", "Session-Main"}

	// All required present (case insensitive)
	if !hasAllTags(beadTags, []string{"oathkeeper", "temporal"}) {
		t.Fatal("expected true when all required tags present")
	}

	// Case-insensitive matching
	if !hasAllTags(beadTags, []string{"session-main"}) {
		t.Fatal("expected case-insensitive match for session-main")
	}

	// Missing tag
	if hasAllTags(beadTags, []string{"oathkeeper", "followup"}) {
		t.Fatal("expected false when required tag missing")
	}

	// Empty required returns true
	if !hasAllTags(beadTags, nil) {
		t.Fatal("expected true with nil required")
	}
	if !hasAllTags(beadTags, []string{}) {
		t.Fatal("expected true with empty required")
	}

	// Empty bead tags
	if hasAllTags(nil, []string{"oathkeeper"}) {
		t.Fatal("expected false when bead has no tags")
	}

	// Whitespace/empty bead tags ignored
	if hasAllTags([]string{"", " ", "oathkeeper"}, []string{"oathkeeper"}) != true {
		t.Fatal("expected true skipping empty bead tags")
	}
}

func TestWriteJSON(t *testing.T) {
	// Create a temp file to test writeJSON output
	tmpFile, err := os.CreateTemp("", "oathkeeper-test-*.json")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	payload := map[string]interface{}{
		"status": "ok",
		"count":  42,
	}
	writeJSON(tmpFile, payload)
	tmpFile.Close()

	data, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("read temp file: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if decoded["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", decoded["status"])
	}
	if decoded["count"] != float64(42) {
		t.Fatalf("expected count 42, got %v", decoded["count"])
	}
}

func TestBuildStatsSummaryEmptyStatus(t *testing.T) {
	// Beads with empty/whitespace status should be counted as "unknown"
	now := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
	list := []beads.Bead{
		{ID: "br-1", Status: "", Tags: []string{"oathkeeper"}, CreatedAt: now},
		{ID: "br-2", Status: "  ", Tags: []string{"oathkeeper"}, CreatedAt: now},
	}

	summary := buildStatsSummary(list, now)
	if summary.Total != 2 {
		t.Fatalf("total = %d, want 2", summary.Total)
	}
	if summary.ByStatus["unknown"] != 2 {
		t.Fatalf("expected 2 unknown status, got %v", summary.ByStatus)
	}
	// None should be counted as open/resolved/backed/etc
	if summary.Open != 0 || summary.Resolved != 0 || summary.Backed != 0 {
		t.Fatalf("expected all named counters 0, got open=%d resolved=%d backed=%d",
			summary.Open, summary.Resolved, summary.Backed)
	}
}

func TestBuildStatsSummaryZeroCreatedAt(t *testing.T) {
	// Beads with zero CreatedAt should not affect oldest open or recent count
	now := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
	list := []beads.Bead{
		{ID: "br-1", Status: "open", Tags: []string{"oathkeeper"}, CreatedAt: time.Time{}},
	}

	summary := buildStatsSummary(list, now)
	if summary.Open != 1 {
		t.Fatalf("open = %d, want 1", summary.Open)
	}
	if summary.OldestOpenAgeSeconds != 0 {
		t.Fatalf("oldest_open_age_seconds = %d, want 0 (zero createdAt)", summary.OldestOpenAgeSeconds)
	}
	if summary.Recent24h != 0 {
		t.Fatalf("recent_24h = %d, want 0 (zero createdAt)", summary.Recent24h)
	}
}

func TestFirstCategoryTag(t *testing.T) {
	tests := []struct {
		name string
		tags []string
		want string
	}{
		{"normal category", []string{"oathkeeper", "temporal"}, "temporal"},
		{"skip oathkeeper", []string{"oathkeeper"}, ""},
		{"skip session tag", []string{"oathkeeper", "session-main", "temporal"}, "temporal"},
		{"empty tags", []string{}, ""},
		{"nil tags", nil, ""},
		{"only session and oathkeeper", []string{"oathkeeper", "session-abc"}, ""},
		{"whitespace tag", []string{"  ", "oathkeeper", "followup"}, "followup"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstCategoryTag(tt.tags)
			if got != tt.want {
				t.Fatalf("firstCategoryTag(%v) = %q, want %q", tt.tags, got, tt.want)
			}
		})
	}
}

func TestParseStatsArgsDashboardOutputConflict(t *testing.T) {
	// --output requires --export, so this fails on that check first.
	// Test with --export to get to the --dashboard + --output check.
	_, err := parseStatsArgs([]string{"--dashboard", "/tmp/d.html", "--export", "csv", "--output", "/tmp/o.csv"})
	if err == nil {
		t.Fatal("expected --dashboard + --export to fail")
	}
	// The --dashboard + --export check fires before --dashboard + --output
	if !strings.Contains(err.Error(), "--dashboard cannot be combined with --export") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseStatsArgsUnexpectedArgs(t *testing.T) {
	_, err := parseStatsArgs([]string{"extra-arg"})
	if err == nil {
		t.Fatal("expected unexpected argument to fail")
	}
	if !strings.Contains(err.Error(), "unexpected argument") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSortedMapKeys(t *testing.T) {
	m := map[string]int{"banana": 1, "apple": 2, "cherry": 3}
	keys := sortedMapKeys(m)
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	if keys[0] != "apple" || keys[1] != "banana" || keys[2] != "cherry" {
		t.Fatalf("expected sorted keys, got %v", keys)
	}

	// Empty map
	empty := sortedMapKeys(map[string]int{})
	if len(empty) != 0 {
		t.Fatalf("expected 0 keys for empty map, got %d", len(empty))
	}
}

func TestExtractGlobalFlagsEmptyConfigValue(t *testing.T) {
	_, _, _, err := extractGlobalFlags([]string{"--config", "  "})
	if err == nil {
		t.Fatal("expected error for whitespace config value")
	}
	if !strings.Contains(err.Error(), "--config cannot be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}
