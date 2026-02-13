package formatter

import (
	"strings"
	"testing"
	"time"

	"github.com/perttulands/oathkeeper/pkg/storage"
)

func sampleCommitment(id, status string, mechanisms []string) storage.Commitment {
	now := time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC)
	expires := now.Add(5 * time.Minute)
	lastChecked := now.Add(1 * time.Minute)
	return storage.Commitment{
		ID:          id,
		DetectedAt:  now,
		Source:      "athena-01",
		MessageID:   "msg-" + id,
		Text:        "I'll check back in 5 minutes",
		Category:    storage.CategoryTemporal,
		BackedBy:    mechanisms,
		Status:      status,
		ExpiresAt:   &expires,
		LastChecked: &lastChecked,
		AlertCount:  0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func TestFormatTableHeader(t *testing.T) {
	out := FormatTable(nil)
	if !strings.Contains(out, "ID") {
		t.Error("table should contain ID header")
	}
	if !strings.Contains(out, "BACKED BY") {
		t.Error("table should contain BACKED BY header")
	}
	if !strings.Contains(out, "STATUS") {
		t.Error("table should contain STATUS header")
	}
}

func TestFormatTableNoMechanisms(t *testing.T) {
	c := sampleCommitment("a1b2c3", storage.StatusAlerted, []string{})
	out := FormatTable([]storage.Commitment{c})
	if !strings.Contains(out, "(none)") {
		t.Errorf("empty mechanisms should show (none), got:\n%s", out)
	}
}

func TestFormatTableWithMechanisms(t *testing.T) {
	c := sampleCommitment("d4e5f6", storage.StatusBacked, []string{"cron:abc123"})
	out := FormatTable([]storage.Commitment{c})
	if !strings.Contains(out, "cron:abc123") {
		t.Errorf("table should show mechanism, got:\n%s", out)
	}
}

func TestFormatTableMultipleMechanisms(t *testing.T) {
	c := sampleCommitment("g7h8i9", storage.StatusBacked, []string{"cron:abc123", "bead:build-watcher"})
	out := FormatTable([]storage.Commitment{c})
	if !strings.Contains(out, "cron:abc123") {
		t.Errorf("table should show first mechanism, got:\n%s", out)
	}
	if !strings.Contains(out, "bead:build-watcher") {
		t.Errorf("table should show second mechanism, got:\n%s", out)
	}
}

func TestFormatTableMultipleRows(t *testing.T) {
	commitments := []storage.Commitment{
		sampleCommitment("aaa", storage.StatusAlerted, []string{}),
		sampleCommitment("bbb", storage.StatusBacked, []string{"cron:xyz"}),
	}
	out := FormatTable(commitments)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// Header + 2 data rows minimum
	if len(lines) < 3 {
		t.Errorf("expected at least 3 lines (header + 2 rows), got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(out, "aaa") {
		t.Error("should contain first commitment ID")
	}
	if !strings.Contains(out, "bbb") {
		t.Error("should contain second commitment ID")
	}
}

func TestFormatTableTruncatesID(t *testing.T) {
	c := sampleCommitment("abcdef1234567890", storage.StatusAlerted, []string{})
	out := FormatTable([]storage.Commitment{c})
	// ID should be truncated to 8 chars with ellipsis (5 chars + "...")
	if !strings.Contains(out, "abcde...") {
		t.Errorf("table should show truncated ID, got:\n%s", out)
	}
}

func TestFormatTableTruncatesText(t *testing.T) {
	c := sampleCommitment("txt1", storage.StatusAlerted, []string{})
	c.Text = "I'll check back on the server and ensure everything is running smoothly and report back with full details on the status"
	out := FormatTable([]storage.Commitment{c})
	// Text should be truncated with ellipsis
	if !strings.Contains(out, "...") {
		t.Errorf("long text should be truncated with ellipsis, got:\n%s", out)
	}
}

func TestFormatDetailNoMechanisms(t *testing.T) {
	c := sampleCommitment("det1", storage.StatusAlerted, []string{})
	out := FormatDetail(c)
	if !strings.Contains(out, "det1") {
		t.Error("detail should contain full ID")
	}
	if !strings.Contains(out, "Mechanisms") {
		t.Error("detail should have Mechanisms section")
	}
	if !strings.Contains(out, "(none)") {
		t.Errorf("no mechanisms should show (none), got:\n%s", out)
	}
}

func TestFormatDetailWithMechanisms(t *testing.T) {
	c := sampleCommitment("det2", storage.StatusBacked, []string{"cron:abc123", "bead:build-watcher"})
	out := FormatDetail(c)
	if !strings.Contains(out, "cron:abc123") {
		t.Errorf("detail should show first mechanism, got:\n%s", out)
	}
	if !strings.Contains(out, "bead:build-watcher") {
		t.Errorf("detail should show second mechanism, got:\n%s", out)
	}
}

func TestFormatDetailShowsAllFields(t *testing.T) {
	c := sampleCommitment("det3", storage.StatusBacked, []string{"cron:xyz"})
	out := FormatDetail(c)

	fields := []string{"ID", "Source", "Category", "Status", "Text", "Detected", "Expires", "Mechanisms"}
	for _, f := range fields {
		if !strings.Contains(out, f) {
			t.Errorf("detail should contain field %q, got:\n%s", f, out)
		}
	}
}

func TestFormatDetailNilExpiresAt(t *testing.T) {
	c := sampleCommitment("det4", storage.StatusUnverified, []string{})
	c.ExpiresAt = nil
	out := FormatDetail(c)
	if !strings.Contains(out, "(none)") || !strings.Contains(out, "Expires") {
		t.Errorf("nil ExpiresAt should show (none), got:\n%s", out)
	}
}

func TestFormatDetailShowsAlertCount(t *testing.T) {
	c := sampleCommitment("det5", storage.StatusAlerted, []string{})
	c.AlertCount = 3
	out := FormatDetail(c)
	if !strings.Contains(out, "Alerts") {
		t.Error("detail should show alert count label")
	}
	if !strings.Contains(out, "3") {
		t.Errorf("detail should show alert count 3, got:\n%s", out)
	}
}

func TestFormatMechanismsEmpty(t *testing.T) {
	result := FormatMechanisms([]string{})
	if result != "(none)" {
		t.Errorf("FormatMechanisms([]) = %q, want %q", result, "(none)")
	}
}

func TestFormatMechanismsNil(t *testing.T) {
	result := FormatMechanisms(nil)
	if result != "(none)" {
		t.Errorf("FormatMechanisms(nil) = %q, want %q", result, "(none)")
	}
}

func TestFormatMechanismsSingle(t *testing.T) {
	result := FormatMechanisms([]string{"cron:abc123"})
	if result != "cron:abc123" {
		t.Errorf("FormatMechanisms = %q, want %q", result, "cron:abc123")
	}
}

func TestFormatMechanismsMultiple(t *testing.T) {
	result := FormatMechanisms([]string{"cron:abc123", "bead:build-watcher"})
	if result != "cron:abc123, bead:build-watcher" {
		t.Errorf("FormatMechanisms = %q, want %q", result, "cron:abc123, bead:build-watcher")
	}
}
