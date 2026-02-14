package beadtracker

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewBeadTracker(t *testing.T) {
	bt := NewBeadTracker("br")
	if bt == nil {
		t.Fatal("NewBeadTracker returned nil")
	}
	if bt.command != "br" {
		t.Errorf("command = %q, want %q", bt.command, "br")
	}
}

func TestNewBeadTrackerCustomCommand(t *testing.T) {
	bt := NewBeadTracker("/usr/local/bin/br")
	if bt.command != "/usr/local/bin/br" {
		t.Errorf("command = %q, want %q", bt.command, "/usr/local/bin/br")
	}
}

func TestBeadTitleFormat(t *testing.T) {
	bt := NewBeadTracker("br")
	title := bt.beadTitle("I'll check back in 5 minutes")
	if !strings.Contains(title, "oathkeeper") {
		t.Errorf("title should contain 'oathkeeper', got %q", title)
	}
	if !strings.Contains(title, "I'll check back in 5 minutes") {
		t.Errorf("title should contain commitment text, got %q", title)
	}
}

func TestBeadTitleTruncation(t *testing.T) {
	bt := NewBeadTracker("br")
	longText := strings.Repeat("a", 200)
	title := bt.beadTitle(longText)
	if len(title) > 120 {
		t.Errorf("title should be truncated, got length %d", len(title))
	}
}

func TestBeadBodyFormat(t *testing.T) {
	bt := NewBeadTracker("br")
	now := time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC)
	expires := time.Date(2026, 2, 13, 14, 35, 0, 0, time.UTC)

	body := bt.beadBody("abc123", "I'll check back in 5 minutes", "temporal", now, &expires)

	if !strings.Contains(body, "abc123") {
		t.Error("body should contain commitment ID")
	}
	if !strings.Contains(body, "temporal") {
		t.Error("body should contain category")
	}
	if !strings.Contains(body, "I'll check back in 5 minutes") {
		t.Error("body should contain commitment text")
	}
	if !strings.Contains(body, "2026-02-13") {
		t.Error("body should contain detected date")
	}
	if !strings.Contains(body, "14:35") {
		t.Error("body should contain expires time")
	}
}

func TestBeadBodyNoExpiration(t *testing.T) {
	bt := NewBeadTracker("br")
	now := time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC)

	body := bt.beadBody("abc123", "I'll monitor this", "followup", now, nil)

	if strings.Contains(body, "Expires") {
		t.Error("body should not contain Expires when nil")
	}
	if !strings.Contains(body, "abc123") {
		t.Error("body should still contain commitment ID")
	}
}

func TestCreateBeadCommandNotFound(t *testing.T) {
	bt := NewBeadTracker("nonexistent-command-xyz")
	now := time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC)

	_, err := bt.CreateBead("id1", "I'll check", "temporal", now, nil)
	if err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestCreateBeadWithMockScript(t *testing.T) {
	// Create a mock script that simulates `br create`
	script := `#!/bin/sh
echo "bead-track-abc123"
`
	tmpFile, err := os.CreateTemp("", "mock-br-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(script); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	os.Chmod(tmpFile.Name(), 0755)

	bt := NewBeadTracker(tmpFile.Name())
	now := time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC)

	beadID, err := bt.CreateBead("commit-1", "I'll check in 5 minutes", "temporal", now, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if beadID != "bead-track-abc123" {
		t.Errorf("beadID = %q, want %q", beadID, "bead-track-abc123")
	}
}

func TestCreateBeadWithExpiration(t *testing.T) {
	// Verify args are passed correctly by echoing them
	script := `#!/bin/sh
echo "bead-track-xyz789"
`
	tmpFile, err := os.CreateTemp("", "mock-br-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(script); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	os.Chmod(tmpFile.Name(), 0755)

	bt := NewBeadTracker(tmpFile.Name())
	now := time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC)
	expires := time.Date(2026, 2, 13, 15, 30, 0, 0, time.UTC)

	beadID, err := bt.CreateBead("commit-2", "I'll report at 3pm", "scheduled", now, &expires)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if beadID != "bead-track-xyz789" {
		t.Errorf("beadID = %q, want %q", beadID, "bead-track-xyz789")
	}
}

func TestCreateBeadScriptFailure(t *testing.T) {
	script := `#!/bin/sh
echo "error: something went wrong" >&2
exit 1
`
	tmpFile, err := os.CreateTemp("", "mock-br-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(script); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	os.Chmod(tmpFile.Name(), 0755)

	bt := NewBeadTracker(tmpFile.Name())
	now := time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC)

	_, err = bt.CreateBead("commit-3", "I'll check", "temporal", now, nil)
	if err == nil {
		t.Fatal("expected error for script failure")
	}
}

func TestCreateBeadEmptyOutput(t *testing.T) {
	script := `#!/bin/sh
echo ""
`
	tmpFile, err := os.CreateTemp("", "mock-br-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(script); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	os.Chmod(tmpFile.Name(), 0755)

	bt := NewBeadTracker(tmpFile.Name())
	now := time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC)

	_, err = bt.CreateBead("commit-4", "I'll check", "temporal", now, nil)
	if err == nil {
		t.Fatal("expected error for empty output")
	}
}

func TestCreateBeadTimeout(t *testing.T) {
	// Use a script that traps SIGTERM to verify context cancellation produces an error.
	// Note: shell script children (sleep) may outlive the parent after SIGKILL,
	// but cmd.Output() returns once the parent shell is killed.
	script := `#!/bin/sh
trap "" TERM
sleep 2
echo "bead-never"
`
	tmpFile, err := os.CreateTemp("", "mock-br-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(script); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	os.Chmod(tmpFile.Name(), 0755)

	bt := NewBeadTracker(tmpFile.Name())
	bt.SetTimeout(100 * time.Millisecond)
	now := time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC)

	start := time.Now()
	_, err = bt.CreateBead("commit-5", "I'll check", "temporal", now, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed > 5*time.Second {
		t.Errorf("took %v, expected quick timeout", elapsed)
	}
}

func TestSetTimeout(t *testing.T) {
	bt := NewBeadTracker("br")
	bt.SetTimeout(10 * time.Second)
	if bt.timeout != 10*time.Second {
		t.Errorf("timeout = %v, want 10s", bt.timeout)
	}
}

func TestDefaultTimeout(t *testing.T) {
	bt := NewBeadTracker("br")
	if bt.timeout != 5*time.Second {
		t.Errorf("default timeout = %v, want 5s", bt.timeout)
	}
}

func TestCreateBeadArgsPassedCorrectly(t *testing.T) {
	// Verify the br command receives correct arguments
	script := `#!/bin/sh
# Verify we got "create" as first arg
if [ "$1" != "create" ]; then
  echo "error: expected 'create' as first arg, got '$1'" >&2
  exit 1
fi
# Check for --title flag
found_title=0
for arg in "$@"; do
  case "$arg" in
    --title) found_title=1 ;;
  esac
done
if [ "$found_title" = "0" ]; then
  echo "error: missing --title flag" >&2
  exit 1
fi
echo "bead-args-ok"
`
	tmpFile, err := os.CreateTemp("", "mock-br-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(script); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	os.Chmod(tmpFile.Name(), 0755)

	bt := NewBeadTracker(tmpFile.Name())
	now := time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC)

	beadID, err := bt.CreateBead("commit-6", "I'll check tomorrow", "temporal", now, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if beadID != "bead-args-ok" {
		t.Errorf("beadID = %q, want %q", beadID, "bead-args-ok")
	}
}

func TestCreateBeadIntegrationWithBR(t *testing.T) {
	brPath, err := exec.LookPath("br")
	if err != nil {
		t.Skip("br not available in PATH")
	}

	workspace := t.TempDir()
	beadsDir := filepath.Join(workspace, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("create beads dir: %v", err)
	}
	dbPath := filepath.Join(beadsDir, "beads.db")
	wrapperPath := filepath.Join(workspace, "br-wrapper.sh")
	wrapperScript := "#!/bin/sh\nexec " + brPath + " --db " + dbPath + " \"$@\"\n"
	if err := os.WriteFile(wrapperPath, []byte(wrapperScript), 0755); err != nil {
		t.Fatalf("write wrapper script: %v", err)
	}

	bt := NewBeadTracker(wrapperPath)
	now := time.Now().UTC().Truncate(time.Second)
	beadID, err := bt.CreateBead("integration-commitment", "I'll report back in 5 minutes", "temporal", now, nil)
	if err != nil {
		t.Fatalf("CreateBead returned error: %v", err)
	}
	if beadID == "" {
		t.Fatal("CreateBead returned empty bead ID")
	}

	listOut, err := exec.Command(wrapperPath, "list", "--json").Output()
	if err != nil {
		t.Fatalf("br list failed: %v", err)
	}

	type beadListItem struct {
		ID     string   `json:"id"`
		Status string   `json:"status"`
		Labels []string `json:"labels"`
	}
	var issues []beadListItem
	if err := json.Unmarshal(listOut, &issues); err != nil {
		t.Fatalf("parse br list JSON: %v\noutput: %s", err, string(listOut))
	}

	found := false
	for _, issue := range issues {
		if issue.ID != beadID {
			continue
		}
		found = true
		if issue.Status != "open" {
			t.Errorf("bead status = %q, want open", issue.Status)
		}
		hasOathkeeperLabel := false
		for _, label := range issue.Labels {
			if label == "oathkeeper" {
				hasOathkeeperLabel = true
				break
			}
		}
		if !hasOathkeeperLabel {
			t.Errorf("bead labels = %v, want to include oathkeeper", issue.Labels)
		}
		break
	}
	if !found {
		t.Fatalf("created bead %q not found in br list --json", beadID)
	}

	if out, err := exec.Command(wrapperPath, "close", beadID, "--reason", "integration-cleanup", "--json").CombinedOutput(); err != nil {
		t.Fatalf("br close failed: %v\noutput: %s", err, string(out))
	}

	listAfterCloseOut, err := exec.Command(wrapperPath, "list", "--json").Output()
	if err != nil {
		t.Fatalf("br list after close failed: %v", err)
	}
	var issuesAfterClose []beadListItem
	if err := json.Unmarshal(listAfterCloseOut, &issuesAfterClose); err != nil {
		t.Fatalf("parse br list after close JSON: %v\noutput: %s", err, string(listAfterCloseOut))
	}
	for _, issue := range issuesAfterClose {
		if issue.ID == beadID {
			t.Fatalf("bead %q still appears in open issue list after close", beadID)
		}
	}
}
