package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/perttulands/oathkeeper/pkg/beads"
	"github.com/perttulands/oathkeeper/pkg/detector"
	"github.com/perttulands/oathkeeper/pkg/grace"
)

// AnalyzeRequest is the request payload for POST /api/v2/analyze.
type AnalyzeRequest struct {
	SessionKey string `json:"session_key"`
	Message    string `json:"message"`
	Role       string `json:"role"`
}

// AnalyzeResponse is the response payload for POST /api/v2/analyze.
type AnalyzeResponse struct {
	Commitment bool     `json:"commitment"`
	Category   string   `json:"category,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
	Text       string   `json:"text,omitempty"`
	Resolved   []string `json:"resolved,omitempty"`
}

// V2API exposes v2 oathkeeper endpoints.
type V2API struct {
	detectCommitment func(string) detector.DetectionResult
	autoResolve      func(sessionKey string, message string) ([]string, error)
	scheduleGrace    func(commitmentID string, detectedAt time.Time, callback func(grace.VerificationOutcome))
	now              func() time.Time
}

// NewV2API constructs a v2 API handler from runtime dependencies.
func NewV2API(d *detector.Detector, beadStore *beads.BeadStore, gp *grace.GracePeriod) *V2API {
	v2 := &V2API{
		now: time.Now,
	}

	if d != nil {
		v2.detectCommitment = d.DetectCommitment
	}

	if beadStore != nil {
		v2.autoResolve = beadStore.AutoResolve
	}

	if gp != nil {
		v2.scheduleGrace = gp.Schedule
	}

	return v2
}

// Handler returns an HTTP handler for v2 API routes.
func (v2 *V2API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/analyze", v2.handleAnalyze)
	return mux
}

func (v2 *V2API) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req AnalyzeRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if strings.ToLower(strings.TrimSpace(req.Role)) != "assistant" {
		writeJSON(w, http.StatusOK, AnalyzeResponse{
			Commitment: false,
			Resolved:   []string{},
		})
		return
	}

	result := detector.DetectionResult{}
	if v2 != nil && v2.detectCommitment != nil {
		result = v2.detectCommitment(req.Message)
	}

	if result.IsCommitment {
		detectedAt := time.Now().UTC()
		if v2 != nil && v2.now != nil {
			detectedAt = v2.now().UTC()
		}

		if v2 != nil && v2.scheduleGrace != nil {
			commitmentID := formatAnalyzeCommitmentID(req.SessionKey, detectedAt)
			v2.scheduleGrace(commitmentID, detectedAt, func(grace.VerificationOutcome) {})
		}

		text := strings.TrimSpace(result.CommitmentText)
		if text == "" {
			text = strings.TrimSpace(req.Message)
		}

		writeJSON(w, http.StatusOK, AnalyzeResponse{
			Commitment: true,
			Category:   string(result.Category),
			Confidence: result.Confidence,
			Text:       text,
		})
		return
	}

	resolved := []string{}
	if v2 != nil && v2.autoResolve != nil {
		matches, err := v2.autoResolve(req.SessionKey, req.Message)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("auto resolve: %v", err))
			return
		}
		if len(matches) > 0 {
			resolved = append(resolved, matches...)
		}
	}

	writeJSON(w, http.StatusOK, AnalyzeResponse{
		Commitment: false,
		Resolved:   resolved,
	})
}

func formatAnalyzeCommitmentID(sessionKey string, detectedAt time.Time) string {
	normalized := strings.TrimSpace(strings.ToLower(sessionKey))
	if normalized == "" {
		normalized = "default"
	}
	var b strings.Builder
	for _, ch := range normalized {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			b.WriteRune(ch)
		} else {
			b.WriteRune('-')
		}
	}
	return fmt.Sprintf("v2-%s-%d", b.String(), detectedAt.UnixNano())
}
