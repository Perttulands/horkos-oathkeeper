package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/perttulands/oathkeeper/pkg/beads"
	"github.com/perttulands/oathkeeper/pkg/detector"
	"github.com/perttulands/oathkeeper/pkg/grace"
)

// GraceCallbackContext carries commitment metadata for grace callback handling.
type GraceCallbackContext struct {
	CommitmentID string
	SessionKey   string
	Message      string
	Category     string
	DetectedAt   time.Time
}

// GraceCallbackFunc is called after the grace period expires with verification
// outcome and commitment metadata.
type GraceCallbackFunc func(ctx GraceCallbackContext, outcome grace.VerificationOutcome)

// AnalyzeRequest is the request payload for POST /api/v2/analyze.
type AnalyzeRequest struct {
	SessionKey string `json:"session_key"`
	Message    string `json:"message"`
	Role       string `json:"role"`
}

// FulfilledResponse represents a fulfilled commitment in the analyze response.
type FulfilledResponse struct {
	Text        string `json:"text"`
	FulfilledBy string `json:"fulfilled_by"`
}

// EscalatedResponse represents an escalated commitment type in the analyze response.
type EscalatedResponse struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

// AnalyzeResponse is the response payload for POST /api/v2/analyze.
type AnalyzeResponse struct {
	Commitment bool                `json:"commitment"`
	Category   string              `json:"category,omitempty"`
	Confidence float64             `json:"confidence,omitempty"`
	Text       string              `json:"text,omitempty"`
	Resolved   []string            `json:"resolved,omitempty"`
	Fulfilled  []FulfilledResponse `json:"fulfilled,omitempty"`
	Escalated  []EscalatedResponse `json:"escalated,omitempty"`
}

// sessionBuffer holds the message history for a single session.
type sessionBuffer struct {
	messages []string
}

// V2API exposes v2 oathkeeper endpoints.
type V2API struct {
	detectCommitment  func(string) detector.DetectionResult
	autoResolve       func(sessionKey string, message string) ([]string, error)
	listBeads         func(filter beads.Filter) ([]beads.Bead, error)
	getBead           func(beadID string) (beads.Bead, error)
	resolveBead       func(beadID string, reason string) error
	scheduleGrace     func(commitmentID string, detectedAt time.Time, callback func(grace.VerificationOutcome))
	graceCallback     GraceCallbackFunc
	onResolve         func(beadID string, evidence string)
	now               func() time.Time
	mu                sync.Mutex
	sessions          map[string]*sessionBuffer
	contextWindowSize int
	contextAnalyzer   *detector.ContextAnalyzer
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
		v2.listBeads = beadStore.List
		v2.getBead = beadStore.Get
		v2.resolveBead = beadStore.Resolve
	}

	if gp != nil {
		v2.scheduleGrace = gp.Schedule
	}

	return v2
}

// SetGraceCallback sets the function called after grace period verification completes.
// This is where bead creation and webhook notifications happen for unbacked commitments.
func (v2 *V2API) SetGraceCallback(fn GraceCallbackFunc) {
	v2.graceCallback = fn
}

// SetResolveCallback sets the function called when a commitment bead is resolved,
// either via manual resolve or auto-resolve during analysis.
func (v2 *V2API) SetResolveCallback(fn func(beadID string, evidence string)) {
	v2.onResolve = fn
}

// SetResolveBead overrides the bead resolution function (for testing without a real BeadStore).
func (v2 *V2API) SetResolveBead(fn func(beadID string, reason string) error) {
	v2.resolveBead = fn
}

// SetContextAnalyzer sets the context analyzer and window size for session buffering.
func (v2 *V2API) SetContextAnalyzer(ca *detector.ContextAnalyzer, windowSize int) {
	if windowSize <= 0 {
		windowSize = 5
	}
	v2.contextAnalyzer = ca
	v2.contextWindowSize = windowSize
	v2.sessions = make(map[string]*sessionBuffer)
}

// Handler returns an HTTP handler for v2 API routes.
func (v2 *V2API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/analyze", v2.handleAnalyze)
	mux.HandleFunc("/api/v2/commitments", v2.handleCommitments)
	mux.HandleFunc("/api/v2/commitments/", v2.handleCommitmentByIDOrResolve)
	mux.HandleFunc("/api/v2/stats", v2.handleStats)
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
		log.Printf("analyze: invalid JSON body: %v", err)
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

	// Append to session buffer (under lock), copy buffer for analysis
	var bufferCopy []string
	if v2.sessions != nil {
		v2.mu.Lock()
		sb, ok := v2.sessions[req.SessionKey]
		if !ok {
			sb = &sessionBuffer{}
			v2.sessions[req.SessionKey] = sb
		}
		sb.messages = append(sb.messages, req.Message)
		if v2.contextWindowSize > 0 && len(sb.messages) > v2.contextWindowSize {
			sb.messages = sb.messages[len(sb.messages)-v2.contextWindowSize:]
		}
		bufferCopy = make([]string, len(sb.messages))
		copy(bufferCopy, sb.messages)
		v2.mu.Unlock()
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

		text := strings.TrimSpace(result.CommitmentText)
		if text == "" {
			text = strings.TrimSpace(req.Message)
		}

		category := string(result.Category)

		if v2 != nil && v2.scheduleGrace != nil {
			commitmentID := formatAnalyzeCommitmentID(req.SessionKey, detectedAt)
			meta := GraceCallbackContext{
				CommitmentID: commitmentID,
				SessionKey:   req.SessionKey,
				Message:      text,
				Category:     category,
				DetectedAt:   detectedAt,
			}
			v2.scheduleGrace(commitmentID, detectedAt, func(outcome grace.VerificationOutcome) {
				if v2.graceCallback != nil {
					// Ensure callback context always carries the final commitment ID.
					meta.CommitmentID = outcome.CommitmentID
					v2.graceCallback(meta, outcome)
				}
			})
		}

		// Run context analysis on commitment path too
		resp := AnalyzeResponse{
			Commitment: true,
			Category:   category,
			Confidence: result.Confidence,
			Text:       text,
		}
		v2.addContextResults(&resp, bufferCopy, req.SessionKey)
		writeJSON(w, http.StatusOK, resp)
		return
	}

	resolved := []string{}
	if v2 != nil && v2.autoResolve != nil {
		matches, err := v2.autoResolve(req.SessionKey, req.Message)
		if err != nil {
			be := mapBackendError("auto resolve", err)
			writeErrorWithDetail(w, be.Status, be.Msg, err.Error(), be.Hint)
			return
		}
		if len(matches) > 0 {
			resolved = append(resolved, matches...)
			if v2.onResolve != nil {
				for _, id := range matches {
					go v2.onResolve(id, req.Message)
				}
			}
		}
	}

	resp := AnalyzeResponse{
		Commitment: false,
		Resolved:   resolved,
	}
	v2.addContextResults(&resp, bufferCopy, req.SessionKey)
	writeJSON(w, http.StatusOK, resp)
}

// addContextResults runs ContextAnalyzer on the session buffer and populates
// fulfilled/escalated fields in the response. For fulfilled commitments,
// it also closes matching open beads for the session.
func (v2 *V2API) addContextResults(resp *AnalyzeResponse, bufferCopy []string, sessionKey string) {
	if v2.contextAnalyzer == nil || len(bufferCopy) == 0 {
		return
	}

	ctxResult := v2.contextAnalyzer.Analyze(bufferCopy)

	for _, f := range ctxResult.Fulfilled {
		resp.Fulfilled = append(resp.Fulfilled, FulfilledResponse{
			Text:        f.CommitmentText,
			FulfilledBy: f.FulfilledBy,
		})
	}

	// Close matching open beads for fulfilled commitments
	if len(ctxResult.Fulfilled) > 0 && v2.listBeads != nil && v2.resolveBead != nil {
		openBeads, err := v2.listBeads(beads.Filter{Status: "open"})
		if err == nil {
			for _, f := range ctxResult.Fulfilled {
				for _, b := range openBeads {
					if matchesSession(b.Tags, sessionKey) {
						_ = v2.resolveBead(b.ID, "fulfilled: "+f.FulfilledBy)
						if v2.onResolve != nil {
							go v2.onResolve(b.ID, "fulfilled: "+f.FulfilledBy)
						}
						break
					}
				}
			}
		}
	}

	for _, e := range ctxResult.Escalated {
		resp.Escalated = append(resp.Escalated, EscalatedResponse{
			Category: string(e.Category),
			Count:    e.Count,
		})
	}
}

func matchesSession(tags []string, sessionKey string) bool {
	if sessionKey == "" {
		return true
	}
	expected := "session-" + strings.ToLower(strings.TrimSpace(sessionKey))
	for _, tag := range tags {
		t := strings.ToLower(strings.TrimSpace(tag))
		if t == expected {
			return true
		}
	}
	// No session tag on bead — match any session
	for _, tag := range tags {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(tag)), "session-") {
			return false
		}
	}
	return true
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

type commitmentAPIResponse struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	Tags        []string  `json:"tags"`
	CreatedAt   time.Time `json:"created_at"`
	ClosedAt    time.Time `json:"closed_at,omitempty"`
	CloseReason string    `json:"close_reason,omitempty"`
}

type resolveAPIResponse struct {
	ID       string `json:"id"`
	Resolved bool   `json:"resolved"`
}

type resolveRequest struct {
	Reason string `json:"reason"`
}

type statsAPIResponse struct {
	Total      int            `json:"total"`
	Open       int            `json:"open"`
	Resolved   int            `json:"resolved"`
	ByCategory map[string]int `json:"by_category"`
}

func (v2 *V2API) handleCommitments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if v2 == nil || v2.listBeads == nil {
		writeError(w, http.StatusInternalServerError, "bead store unavailable")
		return
	}

	filter := beads.Filter{
		Status:   strings.TrimSpace(r.URL.Query().Get("status")),
		Category: strings.TrimSpace(r.URL.Query().Get("category")),
	}

	list, err := v2.listBeads(filter)
	if err != nil {
		be := mapBackendError("list commitments", err)
		writeErrorWithDetail(w, be.Status, be.Msg, err.Error(), be.Hint)
		return
	}

	resp := make([]commitmentAPIResponse, 0, len(list))
	for _, bead := range list {
		resp = append(resp, toCommitmentResponse(bead))
	}

	writeJSON(w, http.StatusOK, resp)
}

func (v2 *V2API) handleCommitmentByIDOrResolve(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v2/commitments/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeError(w, http.StatusBadRequest, "missing commitment ID")
		return
	}

	if strings.HasSuffix(path, "/resolve") {
		v2.handleResolveCommitment(w, r, strings.TrimSuffix(path, "/resolve"))
		return
	}

	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if strings.Contains(path, "/") {
		writeError(w, http.StatusNotFound, "commitment not found")
		return
	}

	if v2 == nil || v2.getBead == nil {
		writeError(w, http.StatusInternalServerError, "bead store unavailable")
		return
	}

	bead, err := v2.getBead(path)
	if err != nil {
		be := mapBackendError("get commitment", err)
		writeErrorWithDetail(w, be.Status, be.Msg, err.Error(), be.Hint)
		return
	}

	writeJSON(w, http.StatusOK, toCommitmentResponse(bead))
}

func (v2 *V2API) handleResolveCommitment(w http.ResponseWriter, r *http.Request, beadID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	beadID = strings.Trim(beadID, "/")
	if beadID == "" || strings.Contains(beadID, "/") {
		writeError(w, http.StatusBadRequest, "missing commitment ID")
		return
	}

	var req resolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("resolve: invalid JSON body: %v", err)
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		writeError(w, http.StatusBadRequest, "reason is required")
		return
	}

	if v2 == nil || v2.resolveBead == nil {
		writeError(w, http.StatusInternalServerError, "bead store unavailable")
		return
	}

	if err := v2.resolveBead(beadID, req.Reason); err != nil {
		be := mapBackendError("resolve commitment", err)
		writeErrorWithDetail(w, be.Status, be.Msg, err.Error(), be.Hint)
		return
	}

	if v2.onResolve != nil {
		go v2.onResolve(beadID, req.Reason)
	}

	writeJSON(w, http.StatusOK, resolveAPIResponse{
		ID:       beadID,
		Resolved: true,
	})
}

func (v2 *V2API) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if v2 == nil || v2.listBeads == nil {
		writeError(w, http.StatusInternalServerError, "bead store unavailable")
		return
	}

	list, err := v2.listBeads(beads.Filter{})
	if err != nil {
		be := mapBackendError("list commitments", err)
		writeErrorWithDetail(w, be.Status, be.Msg, err.Error(), be.Hint)
		return
	}

	resp := statsAPIResponse{
		Total:      len(list),
		ByCategory: map[string]int{},
	}
	for _, bead := range list {
		switch strings.ToLower(strings.TrimSpace(bead.Status)) {
		case "open":
			resp.Open++
		case "closed":
			resp.Resolved++
		}

		if category := beadCategory(bead.Tags); category != "" {
			resp.ByCategory[category]++
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func beadCategory(tags []string) string {
	for _, tag := range tags {
		normalized := strings.ToLower(strings.TrimSpace(tag))
		if normalized == "" || normalized == "oathkeeper" || strings.HasPrefix(normalized, "session-") {
			continue
		}
		return normalized
	}
	return ""
}

func toCommitmentResponse(bead beads.Bead) commitmentAPIResponse {
	return commitmentAPIResponse{
		ID:          bead.ID,
		Title:       bead.Title,
		Status:      bead.Status,
		Tags:        bead.Tags,
		CreatedAt:   bead.CreatedAt,
		ClosedAt:    bead.ClosedAt,
		CloseReason: bead.CloseReason,
	}
}
