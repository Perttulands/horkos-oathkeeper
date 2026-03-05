package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/perttulands/horkos-oathkeeper/pkg/api"
	"github.com/perttulands/horkos-oathkeeper/pkg/beads"
	"github.com/perttulands/horkos-oathkeeper/pkg/config"
	"github.com/perttulands/horkos-oathkeeper/pkg/daemon"
	"github.com/perttulands/horkos-oathkeeper/pkg/detector"
	"github.com/perttulands/horkos-oathkeeper/pkg/grace"
	"github.com/perttulands/horkos-oathkeeper/pkg/hooks"
	"github.com/perttulands/horkos-oathkeeper/pkg/ingest"
	"github.com/perttulands/horkos-oathkeeper/pkg/recheck"
	"github.com/perttulands/horkos-oathkeeper/pkg/relaypub"
	"github.com/perttulands/horkos-oathkeeper/pkg/verifier"
)

func startServer(configPath string, extraTags []string, cliDryRun bool) {
	cfg := loadConfig(configPath)
	dryRun := cliDryRun || cfg.General.DryRun
	runtimeState := newRuntimeHealth(cfg.General.ReadinessErrorThreshold, cfg.ReadinessErrorWindowDuration())
	recordRuntimeError := func(stage string, err error) {
		if err == nil {
			return
		}
		runtimeState.RecordFailure(fmt.Errorf("%s: %w", stage, err))
	}

	stateDirs := expandPaths(cfg.Verification.StateDirs)
	memoryDirs := expandPaths(cfg.Verification.MemoryDirs)
	transcriptRoot := config.ExpandPath(cfg.OpenClaw.TranscriptDir)

	// Wire dependencies
	beadStore := beads.NewBeadStore(cfg.Verification.BeadsCommand)
	beadStore.SetDryRun(dryRun)
	det := detector.NewDetectorWithMinConfidence(cfg.Detector.MinConfidence)
	ver := verifier.NewVerifierFromConfig(verifier.Options{
		CronAPIURL:   cfg.OpenClaw.APIURL,
		CronEndpoint: cfg.OpenClaw.CronEndpoint,
		BeadsCommand: cfg.Verification.BeadsCommand,
		StateDirs:    stateDirs,
		MemoryDirs:   memoryDirs,
	})
	// Re-check uses external mechanisms only; oathkeeper bead existence must not
	// self-satisfy commitments.
	recheckVerifier := verifier.NewVerifierFromConfig(verifier.Options{
		CronAPIURL:   cfg.OpenClaw.APIURL,
		CronEndpoint: cfg.OpenClaw.CronEndpoint,
		StateDirs:    stateDirs,
		MemoryDirs:   memoryDirs,
	})

	// Webhook for notifications (optional)
	var webhook *hooks.Webhook
	var resolutionWebhook *hooks.Webhook
	if cfg.Alerts.TelegramWebhook != "" {
		webhook = hooks.NewWebhook(cfg.Alerts.TelegramWebhook)
	}
	if cfg.Alerts.ResolutionWebhook != "" {
		resolutionWebhook = hooks.NewWebhook(cfg.Alerts.ResolutionWebhook)
	} else {
		resolutionWebhook = webhook
	}
	relayPublisher := relaypub.New(relaypub.Config{
		Enabled: cfg.Relay.Enabled,
		Command: cfg.Relay.Command,
		To:      cfg.Relay.To,
		From:    cfg.Relay.From,
		Timeout: cfg.RelayTimeoutDuration(),
		Retries: cfg.Relay.Retries,
	})

	gracePeriod := grace.New(cfg.GracePeriodDuration(), func(detectedAt time.Time) (*grace.VerificationOutcome, error) {
		result, err := ver.Verify(detectedAt)
		if err != nil {
			return &grace.VerificationOutcome{IsBacked: false}, fmt.Errorf("verify mechanisms: %w", err)
		}
		return &grace.VerificationOutcome{
			IsBacked:   result.IsBacked,
			Mechanisms: result.Mechanisms,
		}, nil
	})

	v2 := api.NewV2API(det, beadStore, gracePeriod)

	// Wire context analyzer for session-aware fulfillment detection
	ca := detector.NewContextAnalyzerWithMinConfidence(cfg.General.ContextWindowSize, cfg.Detector.MinConfidence)
	v2.SetContextAnalyzer(ca, cfg.General.ContextWindowSize)

	// Set the grace period callback to create beads and fire webhooks
	v2.SetGraceCallback(func(meta api.GraceCallbackContext, outcome grace.VerificationOutcome) {
		if outcome.IsBacked {
			return
		}
		if dryRun {
			log.Printf("dry-run: would create bead for unbacked commitment %s", meta.CommitmentID)
			return
		}
		// Create a bead for the unbacked commitment
		beadID, err := beadStore.Create(beads.CommitmentInfo{
			Text:       meta.Message,
			Category:   meta.Category,
			SessionKey: meta.SessionKey,
			DetectedAt: meta.DetectedAt,
			Tags:       extraTags,
		})
		if err != nil {
			recordRuntimeError("create unbacked bead", err)
			log.Printf("failed to create bead for %s: %v", meta.CommitmentID, err)
			return
		}
		log.Printf("created bead %s for unbacked commitment %s", beadID, meta.CommitmentID)

		// Fire webhook notification
		// REASON: Notification delivery is best-effort; bead creation remains the source of truth.
		if webhook != nil {
			if err := webhook.NotifyUnbacked(beadID, meta.Message, meta.Category); err != nil {
				recordRuntimeError("notify unbacked webhook", err)
				log.Printf("webhook notification failed: %v", fmt.Errorf("notify unbacked bead %s: %w", beadID, err))
			}
		}
		if err := relayPublisher.NotifyUnbackedWithContext(beadID, meta.Message, meta.Category, meta.SessionKey, meta.CommitmentID); err != nil {
			recordRuntimeError("notify unbacked relay", err)
			log.Printf("relay notification failed: %v", fmt.Errorf("publish unbacked bead %s: %w", beadID, err))
		}
	})

	notifyResolved := func(beadID, evidence string) {
		if dryRun {
			log.Printf("dry-run: would notify resolution for %s", beadID)
			return
		}
		if resolutionWebhook != nil {
			// REASON: Resolution notification failures must not block state transitions.
			if err := resolutionWebhook.NotifyResolved(beadID, evidence); err != nil {
				recordRuntimeError("notify resolved webhook", err)
				log.Printf("resolve webhook failed: %v", fmt.Errorf("notify resolved bead %s: %w", beadID, err))
			}
		}
		if err := relayPublisher.NotifyResolved(beadID, evidence); err != nil {
			recordRuntimeError("notify resolved relay", err)
			log.Printf("resolve relay publish failed: %v", fmt.Errorf("publish resolved bead %s: %w", beadID, err))
		}
	}

	// Set the resolve callback to fire webhooks and relay notifications when beads are resolved.
	v2.SetResolveCallback(notifyResolved)

	addr := cfg.Server.Addr
	if addr == "" {
		addr = ":9876"
	}
	analyzeURL := localAnalyzeURL(addr)
	mux := http.NewServeMux()

	// Register v2 API routes
	v2Handler := v2.Handler()
	mux.Handle("/api/v2/", v2Handler)
	mux.Handle("/api/v2/analyze", v2Handler)
	mux.Handle("/api/v2/stats", v2Handler)
	mux.Handle("/api/v2/commitments", v2Handler)

	// Health endpoints
	healthHandler := api.NewHealthHandler()
	readyHandler := newRuntimeReadinessHandler(
		api.NewReadinessHandler(cfg.Verification.BeadsCommand),
		runtimeState,
	)
	mux.Handle("/healthz", healthHandler)
	mux.Handle("/readyz", readyHandler)

	alertCounts := map[string]int{}
	var alertCountsMu sync.Mutex
	rechecker := recheck.New(recheck.Config{
		Interval:  cfg.RecheckIntervalDuration(),
		MaxAlerts: cfg.General.MaxAlerts,
		FetchFunc: func() ([]recheck.TrackedCommitment, error) {
			list, err := beadStore.List(beads.Filter{Status: "open"})
			if err != nil {
				return nil, fmt.Errorf("list open commitments: %w", err)
			}

			now := time.Now().UTC()
			items := make([]recheck.TrackedCommitment, 0, len(list))
			for _, b := range list {
				alertCountsMu.Lock()
				alertCount := alertCounts[b.ID]
				alertCountsMu.Unlock()

				status := recheck.StatusUnverified
				if alertCount > 0 {
					status = recheck.StatusAlerted
				}

				var expiresAt *time.Time
				if cfg.Storage.AutoExpireHours > 0 && !b.CreatedAt.IsZero() {
					t := b.CreatedAt.UTC().Add(time.Duration(cfg.Storage.AutoExpireHours) * time.Hour)
					expiresAt = &t
				}

				detectedAt := b.CreatedAt.UTC()
				if detectedAt.IsZero() {
					detectedAt = now
				}

				items = append(items, recheck.TrackedCommitment{
					ID:         b.ID,
					Status:     status,
					DetectedAt: detectedAt,
					ExpiresAt:  expiresAt,
					AlertCount: alertCount,
				})
			}
			return items, nil
		},
		VerifyFunc: func(detectedAt time.Time) (bool, []string, error) {
			if detectedAt.IsZero() {
				detectedAt = time.Now().UTC()
			}
			result, err := recheckVerifier.Verify(detectedAt)
			if err != nil {
				return false, nil, fmt.Errorf("recheck verify mechanisms: %w", err)
			}
			mechanisms := filterMechanisms(result.Mechanisms)
			return len(mechanisms) > 0, mechanisms, nil
		},
		UpdateFunc: func(req recheck.UpdateRequest) error {
			if req.IncrementAlert {
				alertCountsMu.Lock()
				alertCounts[req.CommitmentID]++
				alertCountsMu.Unlock()
			}

			switch req.NewStatus {
			case recheck.StatusBacked:
				reason := buildRecheckReason("backed", req.Mechanisms)
				if dryRun {
					log.Printf("dry-run: would resolve %s (%s)", req.CommitmentID, reason)
					return nil
				}
				if err := beadStore.Resolve(req.CommitmentID, reason); err != nil {
					return fmt.Errorf("resolve backed commitment %s: %w", req.CommitmentID, err)
				}
				alertCountsMu.Lock()
				delete(alertCounts, req.CommitmentID)
				alertCountsMu.Unlock()
				notifyResolved(req.CommitmentID, reason)
			case recheck.StatusExpired:
				reason := "expired without backing mechanism"
				if dryRun {
					log.Printf("dry-run: would close expired commitment %s", req.CommitmentID)
					return nil
				}
				if err := beadStore.Resolve(req.CommitmentID, reason); err != nil {
					return fmt.Errorf("resolve expired commitment %s: %w", req.CommitmentID, err)
				}
				alertCountsMu.Lock()
				delete(alertCounts, req.CommitmentID)
				alertCountsMu.Unlock()
				notifyResolved(req.CommitmentID, reason)
			}
			return nil
		},
		AlertFunc: func(c recheck.TrackedCommitment) error {
			if dryRun {
				log.Printf("dry-run: would send recheck alert for %s", c.ID)
				return nil
			}

			bead, err := beadStore.Get(c.ID)
			if err != nil {
				return fmt.Errorf("get bead %s: %w", c.ID, err)
			}

			text := normalizeCommitmentText(bead.Title)
			category := detectCategoryFromTags(bead.Tags)
			sessionKey := extractSessionFromTags(bead.Tags)
			if sessionKey == "" {
				sessionKey = "unknown-session"
			}

			if webhook != nil {
				if err := webhook.NotifyUnbacked(c.ID, text, category); err != nil {
					recordRuntimeError("notify recheck webhook", err)
					log.Printf("recheck webhook notification failed: %v", fmt.Errorf("notify unbacked bead %s: %w", c.ID, err))
				}
			}
			if err := relayPublisher.NotifyUnbackedWithContext(
				c.ID,
				text,
				category,
				sessionKey,
				fmt.Sprintf("recheck-%s-%d", c.ID, time.Now().UTC().UnixNano()),
			); err != nil {
				recordRuntimeError("notify recheck relay", err)
				log.Printf("recheck relay notification failed: %v", fmt.Errorf("publish unbacked bead %s: %w", c.ID, err))
			}
			return nil
		},
		ErrorFunc: func(err error) {
			recordRuntimeError("recheck loop", err)
			log.Printf("recheck: %v", err)
		},
	})
	rechecker.Start()

	var transcriptPoller *ingest.TranscriptPoller
	if cfg.General.MonitorTranscripts && strings.TrimSpace(transcriptRoot) != "" {
		transcriptPoller = ingest.NewTranscriptPoller(transcriptRoot, cfg.TranscriptPollIntervalDuration(), func(m ingest.Message) error {
			err := dispatchAnalyze(analyzeURL, api.AnalyzeRequest{
				SessionKey: m.SessionKey,
				Message:    m.Text,
				Role:       m.Role,
			})
			if err != nil {
				recordRuntimeError("transcript analyze dispatch", err)
			}
			return err
		})
		transcriptPoller.Start()
		log.Printf("transcript monitor enabled: root=%s interval=%s", transcriptRoot, cfg.TranscriptPollIntervalDuration())
	}

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	d := daemon.New(daemon.Config{
		ShutdownTimeout: 10 * time.Second,
		OnStart: func(ctx context.Context) error {
			fmt.Printf("Oathkeeper listening on %s\n", addr)

			errCh := make(chan error, 1)
			go func() {
				errCh <- server.ListenAndServe()
			}()

			select {
			case err := <-errCh:
				if err == http.ErrServerClosed {
					return nil
				}
				return err
			case <-ctx.Done():
				shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := server.Shutdown(shutCtx); err != nil && err != http.ErrServerClosed {
					log.Printf("server shutdown failed: %v", err)
				}
				return nil
			}
		},
		OnStop: func() {
			if transcriptPoller != nil {
				transcriptPoller.Stop()
			}
			rechecker.Stop()
			gracePeriod.Stop()
			fmt.Println("Oathkeeper stopped.")
		},
	})

	if err := d.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", fmt.Errorf("run daemon: %w", err))
		os.Exit(1)
	}
}

func dispatchAnalyze(analyzeURL string, req api.AnalyzeRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal analyze payload: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, analyzeURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build analyze request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("post analyze request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		payload, readErr := io.ReadAll(io.LimitReader(resp.Body, 2048))
		if readErr != nil {
			return fmt.Errorf("read analyze error response: %w", readErr)
		}
		return fmt.Errorf("analyze failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return fmt.Errorf("drain analyze response: %w", err)
	}
	return nil
}

func localAnalyzeURL(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		addr = ":9876"
	}

	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return strings.TrimRight(addr, "/") + "/api/v2/analyze"
	}
	if strings.HasPrefix(addr, ":") {
		return "http://127.0.0.1" + addr + "/api/v2/analyze"
	}
	return "http://" + strings.TrimRight(addr, "/") + "/api/v2/analyze"
}

func expandPaths(paths []string) []string {
	expanded := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		expanded = append(expanded, config.ExpandPath(path))
	}
	return expanded
}

func normalizeCommitmentText(title string) string {
	title = strings.TrimSpace(title)
	title = strings.TrimPrefix(title, "oathkeeper:")
	return strings.TrimSpace(title)
}

func detectCategoryFromTags(tags []string) string {
	known := map[string]struct{}{
		"temporal":          {},
		"scheduled":         {},
		"followup":          {},
		"conditional":       {},
		"untracked_problem": {},
		"speculative":       {},
		"weak_commitment":   {},
	}
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if _, ok := known[tag]; ok {
			return tag
		}
	}
	return "temporal"
}

func extractSessionFromTags(tags []string) string {
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if !strings.HasPrefix(strings.ToLower(tag), "session-") {
			continue
		}
		return strings.TrimPrefix(tag, "session-")
	}
	return ""
}

func buildRecheckReason(status string, mechanisms []string) string {
	switch status {
	case "backed":
		if len(mechanisms) == 0 {
			return "resolved by periodic re-check: backing mechanism detected"
		}
		return fmt.Sprintf("resolved by periodic re-check: %s", strings.Join(mechanisms, ", "))
	default:
		return "resolved by periodic re-check"
	}
}

func filterMechanisms(mechanisms []string) []string {
	filtered := make([]string, 0, len(mechanisms))
	for _, mechanism := range mechanisms {
		mechanism = strings.TrimSpace(mechanism)
		if mechanism == "" {
			continue
		}
		filtered = append(filtered, mechanism)
	}
	return filtered
}

type runtimeFailure struct {
	at     time.Time
	detail string
}

type runtimeHealth struct {
	mu        sync.Mutex
	threshold int
	window    time.Duration
	failures  []runtimeFailure
}

func newRuntimeHealth(threshold int, window time.Duration) *runtimeHealth {
	if threshold <= 0 {
		threshold = 1
	}
	if window <= 0 {
		window = 5 * time.Minute
	}
	return &runtimeHealth{
		threshold: threshold,
		window:    window,
		failures:  make([]runtimeFailure, 0, threshold+4),
	}
}

func (r *runtimeHealth) RecordFailure(err error) {
	if r == nil || err == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	r.pruneLocked(now)
	r.failures = append(r.failures, runtimeFailure{
		at:     now,
		detail: strings.TrimSpace(err.Error()),
	})
	if len(r.failures) > 2048 {
		r.failures = append([]runtimeFailure(nil), r.failures[len(r.failures)-1024:]...)
	}
}

func (r *runtimeHealth) Ready() (bool, string) {
	if r == nil {
		return true, ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	r.pruneLocked(now)
	if len(r.failures) < r.threshold {
		return true, ""
	}
	last := r.failures[len(r.failures)-1]
	return false, fmt.Sprintf(
		"runtime errors threshold exceeded: %d in %s (last: %s)",
		len(r.failures),
		r.window,
		last.detail,
	)
}

func (r *runtimeHealth) pruneLocked(now time.Time) {
	if len(r.failures) == 0 {
		return
	}
	cutoff := now.Add(-r.window)
	idx := 0
	for idx < len(r.failures) && r.failures[idx].at.Before(cutoff) {
		idx++
	}
	if idx > 0 {
		r.failures = append([]runtimeFailure(nil), r.failures[idx:]...)
	}
}

type runtimeReadinessHandler struct {
	base  http.Handler
	state *runtimeHealth
}

func newRuntimeReadinessHandler(base http.Handler, state *runtimeHealth) http.Handler {
	return &runtimeReadinessHandler{
		base:  base,
		state: state,
	}
}

func (h *runtimeReadinessHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if ready, reason := h.state.Ready(); !ready {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"not ready","error":%q}`, reason)
			return
		}
	}

	if h.base == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"status":"not ready","error":"base readiness handler unavailable"}`)
		return
	}
	h.base.ServeHTTP(w, r)
}
