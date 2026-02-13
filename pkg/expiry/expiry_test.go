package expiry

import (
	"testing"
	"time"
)

func TestParseMinutes(t *testing.T) {
	ref := time.Date(2026, 2, 13, 14, 0, 0, 0, time.UTC)
	tests := []struct {
		text string
		want time.Duration
	}{
		{"I'll check back in 5 minutes", 5 * time.Minute},
		{"I'll check in 10 minutes", 10 * time.Minute},
		{"I'll follow up in 30 minutes", 30 * time.Minute},
		{"I'll check back in 1 minute", 1 * time.Minute},
	}
	for _, tt := range tests {
		got := ComputeExpiresAt(tt.text, ref)
		want := ref.Add(tt.want)
		if !got.Equal(want) {
			t.Errorf("ComputeExpiresAt(%q) = %v, want %v", tt.text, got, want)
		}
	}
}

func TestParseHours(t *testing.T) {
	ref := time.Date(2026, 2, 13, 14, 0, 0, 0, time.UTC)
	tests := []struct {
		text string
		want time.Duration
	}{
		{"I'll check back in 2 hours", 2 * time.Hour},
		{"I'll follow up in 1 hour", 1 * time.Hour},
	}
	for _, tt := range tests {
		got := ComputeExpiresAt(tt.text, ref)
		want := ref.Add(tt.want)
		if !got.Equal(want) {
			t.Errorf("ComputeExpiresAt(%q) = %v, want %v", tt.text, got, want)
		}
	}
}

func TestParseSeconds(t *testing.T) {
	ref := time.Date(2026, 2, 13, 14, 0, 0, 0, time.UTC)
	got := ComputeExpiresAt("I'll check back in 30 seconds", ref)
	want := ref.Add(30 * time.Second)
	if !got.Equal(want) {
		t.Errorf("ComputeExpiresAt(30 seconds) = %v, want %v", got, want)
	}
}

func TestParseTomorrow(t *testing.T) {
	ref := time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC)
	got := ComputeExpiresAt("I'll check tomorrow", ref)
	want := time.Date(2026, 2, 14, 23, 59, 59, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("ComputeExpiresAt(tomorrow) = %v, want %v", got, want)
	}
}

func TestParseSoon(t *testing.T) {
	ref := time.Date(2026, 2, 13, 14, 0, 0, 0, time.UTC)
	got := ComputeExpiresAt("I'll check on this soon", ref)
	want := ref.Add(1 * time.Hour)
	if !got.Equal(want) {
		t.Errorf("ComputeExpiresAt(soon) = %v, want %v", got, want)
	}
}

func TestParseLaterToday(t *testing.T) {
	ref := time.Date(2026, 2, 13, 14, 0, 0, 0, time.UTC)
	got := ComputeExpiresAt("I'll get to that later today", ref)
	want := time.Date(2026, 2, 13, 23, 59, 59, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("ComputeExpiresAt(later today) = %v, want %v", got, want)
	}
}

func TestParseDefault(t *testing.T) {
	ref := time.Date(2026, 2, 13, 14, 0, 0, 0, time.UTC)
	// No recognizable time reference — use default 24 hours
	got := ComputeExpiresAt("I'll handle it", ref)
	want := ref.Add(DefaultExpiration)
	if !got.Equal(want) {
		t.Errorf("ComputeExpiresAt(no time ref) = %v, want %v", got, want)
	}
}

func TestDefaultExpirationConstant(t *testing.T) {
	if DefaultExpiration != 24*time.Hour {
		t.Errorf("DefaultExpiration = %v, want 24h", DefaultExpiration)
	}
}

func TestCaseInsensitive(t *testing.T) {
	ref := time.Date(2026, 2, 13, 14, 0, 0, 0, time.UTC)
	got := ComputeExpiresAt("I'll check TOMORROW morning", ref)
	want := time.Date(2026, 2, 14, 23, 59, 59, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("ComputeExpiresAt(TOMORROW) = %v, want %v", got, want)
	}
}

func TestDurationPrecedenceOverVague(t *testing.T) {
	// If text has both specific duration and vague reference, prefer specific
	ref := time.Date(2026, 2, 13, 14, 0, 0, 0, time.UTC)
	got := ComputeExpiresAt("I'll check back soon, in about 15 minutes", ref)
	want := ref.Add(15 * time.Minute)
	if !got.Equal(want) {
		t.Errorf("ComputeExpiresAt(soon + 15 min) = %v, want %v", got, want)
	}
}
