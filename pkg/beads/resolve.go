package beads

import (
	"fmt"
	"strings"
)

var autoResolveIndicators = []string{
	"i checked",
	"done",
	"completed",
	"here are the results",
}

// Resolve closes a bead and records the provided evidence as the close reason.
func (bs *BeadStore) Resolve(beadID string, evidence string) error {
	if strings.TrimSpace(beadID) == "" {
		return fmt.Errorf("bead ID cannot be empty")
	}
	if strings.TrimSpace(evidence) == "" {
		return fmt.Errorf("evidence cannot be empty")
	}
	return bs.Close(beadID, evidence)
}

// AutoResolve closes matching open oathkeeper beads for a session when message
// text indicates the commitment was fulfilled.
func (bs *BeadStore) AutoResolve(sessionKey string, message string) ([]string, error) {
	if sessionTag(sessionKey) == "" {
		return []string{}, nil
	}
	if !containsResolutionIndicator(message) {
		return []string{}, nil
	}

	openBeads, err := bs.List(Filter{Status: "open"})
	if err != nil {
		return nil, err
	}

	session := sessionTag(sessionKey)
	reason := strings.TrimSpace(message)
	resolved := make([]string, 0, len(openBeads))
	for _, bead := range openBeads {
		if !containsTag(bead.Tags, session) {
			continue
		}
		if err := bs.Resolve(bead.ID, reason); err != nil {
			return resolved, err
		}
		resolved = append(resolved, bead.ID)
	}
	return resolved, nil
}

func containsResolutionIndicator(message string) bool {
	normalized := strings.ToLower(strings.TrimSpace(message))
	if normalized == "" {
		return false
	}

	for _, indicator := range autoResolveIndicators {
		if strings.Contains(normalized, indicator) {
			return true
		}
	}
	return false
}
