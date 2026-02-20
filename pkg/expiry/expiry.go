package expiry

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DefaultExpiration is the fallback expiration for commitments without recognizable time references
const DefaultExpiration = 24 * time.Hour

var (
	// Matches "in N minutes/hours/seconds" (with optional "about/approximately")
	durationRe = regexp.MustCompile(`(?i)\bin\s+(?:about\s+|approximately\s+)?(\d+)\s+(minute|minutes|hour|hours|second|seconds)\b`)

	// Matches "tomorrow"
	tomorrowRe = regexp.MustCompile(`(?i)\btomorrow\b`)

	// Matches "later today"
	laterTodayRe = regexp.MustCompile(`(?i)\blater\s+today\b`)

	// Matches "soon"
	soonRe = regexp.MustCompile(`(?i)\bsoon\b`)
)

// ComputeExpiresAt extracts temporal references from commitment text and returns
// the expiration time. ref is the time of detection. Falls back to DefaultExpiration
// if no recognizable time reference is found.
func ComputeExpiresAt(text string, ref time.Time) time.Time {
	// Most specific first: explicit duration ("in N minutes/hours/seconds")
	if m := durationRe.FindStringSubmatch(text); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return ref.Add(DefaultExpiration)
		}
		unit := strings.ToLower(m[2])
		switch {
		case strings.HasPrefix(unit, "minute"):
			return ref.Add(time.Duration(n) * time.Minute)
		case strings.HasPrefix(unit, "hour"):
			return ref.Add(time.Duration(n) * time.Hour)
		case strings.HasPrefix(unit, "second"):
			return ref.Add(time.Duration(n) * time.Second)
		}
	}

	// "tomorrow" → end of next day
	if tomorrowRe.MatchString(text) {
		next := ref.AddDate(0, 0, 1)
		return time.Date(next.Year(), next.Month(), next.Day(), 23, 59, 59, 0, ref.Location())
	}

	// "later today" → end of current day
	if laterTodayRe.MatchString(text) {
		return time.Date(ref.Year(), ref.Month(), ref.Day(), 23, 59, 59, 0, ref.Location())
	}

	// "soon" → 1 hour
	if soonRe.MatchString(text) {
		return ref.Add(1 * time.Hour)
	}

	// Default: 24 hours
	return ref.Add(DefaultExpiration)
}
