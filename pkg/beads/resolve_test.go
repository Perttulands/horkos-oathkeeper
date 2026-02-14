package beads

import (
	"testing"
	"time"
)

func TestResolveClosesBeadWithEvidence(t *testing.T) {
	store := newTestBeadStore(t)

	id, err := store.Create(CommitmentInfo{
		Text:       "I'll check the deployment logs",
		Category:   "temporal",
		SessionKey: "main",
		DetectedAt: time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	evidence := "I checked the deployment logs and completed the verification."
	if err := store.Resolve(id, evidence); err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	resolved, err := store.Get(id)
	if err != nil {
		t.Fatalf("Get after Resolve failed: %v", err)
	}
	if resolved.Status != "closed" {
		t.Fatalf("Get after Resolve returned status %q, want closed", resolved.Status)
	}
	if resolved.CloseReason != evidence {
		t.Fatalf("Get after Resolve returned close reason %q, want %q", resolved.CloseReason, evidence)
	}
}

func TestAutoResolveResolvesOnlyMatchingSessionBeads(t *testing.T) {
	store := newTestBeadStore(t)

	mainID, err := store.Create(CommitmentInfo{
		Text:       "I'll check the API health endpoint",
		Category:   "followup",
		SessionKey: "main",
		DetectedAt: time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Create main session bead failed: %v", err)
	}

	otherID, err := store.Create(CommitmentInfo{
		Text:       "I'll check the backup cron job",
		Category:   "followup",
		SessionKey: "other-session",
		DetectedAt: time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Create other session bead failed: %v", err)
	}

	resolvedIDs, err := store.AutoResolve("main", "I checked the API health endpoint and here are the results.")
	if err != nil {
		t.Fatalf("AutoResolve failed: %v", err)
	}
	if len(resolvedIDs) != 1 || resolvedIDs[0] != mainID {
		t.Fatalf("AutoResolve resolved %v, want [%s]", resolvedIDs, mainID)
	}

	mainBead, err := store.Get(mainID)
	if err != nil {
		t.Fatalf("Get main bead failed: %v", err)
	}
	if mainBead.Status != "closed" {
		t.Fatalf("main bead status = %q, want closed", mainBead.Status)
	}

	otherBead, err := store.Get(otherID)
	if err != nil {
		t.Fatalf("Get other bead failed: %v", err)
	}
	if otherBead.Status != "open" {
		t.Fatalf("other bead status = %q, want open", otherBead.Status)
	}
}

func TestAutoResolveDoesNotResolveUnrelatedMessages(t *testing.T) {
	store := newTestBeadStore(t)

	id, err := store.Create(CommitmentInfo{
		Text:       "I'll monitor the CPU usage",
		Category:   "followup",
		SessionKey: "main",
		DetectedAt: time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	resolvedIDs, err := store.AutoResolve("main", "Can you check that in 10 minutes?")
	if err != nil {
		t.Fatalf("AutoResolve failed: %v", err)
	}
	if len(resolvedIDs) != 0 {
		t.Fatalf("AutoResolve resolved %v, want none", resolvedIDs)
	}

	bead, err := store.Get(id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if bead.Status != "open" {
		t.Fatalf("bead status = %q, want open", bead.Status)
	}
}
