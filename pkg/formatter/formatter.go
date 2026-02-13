package formatter

import (
	"fmt"
	"strings"

	"github.com/perttulands/oathkeeper/pkg/storage"
)

const (
	maxIDLen   = 8
	maxTextLen = 40
)

// FormatMechanisms formats a backed_by slice for display.
// Returns "(none)" for empty or nil slices, otherwise joins with ", ".
func FormatMechanisms(mechanisms []string) string {
	if len(mechanisms) == 0 {
		return "(none)"
	}
	return strings.Join(mechanisms, ", ")
}

// FormatTable renders a list of commitments as a formatted table with columns:
// ID, SOURCE, CATEGORY, STATUS, TEXT, BACKED BY
func FormatTable(commitments []storage.Commitment) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%-*s  %-12s  %-12s  %-10s  %-*s  %s\n",
		maxIDLen, "ID", "SOURCE", "CATEGORY", "STATUS", maxTextLen, "TEXT", "BACKED BY")

	for _, c := range commitments {
		id := truncate(c.ID, maxIDLen)
		text := truncate(c.Text, maxTextLen)
		mechs := FormatMechanisms(c.BackedBy)

		fmt.Fprintf(&b, "%-*s  %-12s  %-12s  %-10s  %-*s  %s\n",
			maxIDLen, id, c.Source, c.Category, c.Status, maxTextLen, text, mechs)
	}

	return b.String()
}

// FormatDetail renders a single commitment with all fields, including
// the full list of backing mechanisms.
func FormatDetail(c storage.Commitment) string {
	var b strings.Builder

	fmt.Fprintf(&b, "ID:         %s\n", c.ID)
	fmt.Fprintf(&b, "Source:     %s\n", c.Source)
	fmt.Fprintf(&b, "Category:   %s\n", c.Category)
	fmt.Fprintf(&b, "Status:     %s\n", c.Status)
	fmt.Fprintf(&b, "Text:       %s\n", c.Text)
	fmt.Fprintf(&b, "Detected:   %s\n", c.DetectedAt.Format("2006-01-02 15:04:05"))

	if c.ExpiresAt != nil {
		fmt.Fprintf(&b, "Expires:    %s\n", c.ExpiresAt.Format("2006-01-02 15:04:05"))
	} else {
		fmt.Fprintf(&b, "Expires:    (none)\n")
	}

	fmt.Fprintf(&b, "Alerts:     %d\n", c.AlertCount)
	fmt.Fprintf(&b, "Mechanisms: %s\n", FormatMechanisms(c.BackedBy))

	return b.String()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
