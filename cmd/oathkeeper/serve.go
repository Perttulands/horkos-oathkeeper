package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/perttulands/oathkeeper/pkg/api"
	"github.com/perttulands/oathkeeper/pkg/beads"
	"github.com/perttulands/oathkeeper/pkg/daemon"
	"github.com/perttulands/oathkeeper/pkg/detector"
	"github.com/perttulands/oathkeeper/pkg/grace"
	"github.com/perttulands/oathkeeper/pkg/hooks"
	"github.com/perttulands/oathkeeper/pkg/relaypub"
	"github.com/perttulands/oathkeeper/pkg/verifier"
)

func startServer(configPath string, extraTags []string, cliDryRun bool) {
	cfg := loadConfig(configPath)
	dryRun := cliDryRun || cfg.General.DryRun

	// Wire dependencies
	beadStore := beads.NewBeadStore(cfg.Verification.BeadsCommand)
	beadStore.SetDryRun(dryRun)
	det := detector.NewDetectorWithMinConfidence(cfg.Detector.MinConfidence)
	ver := verifier.NewVerifierFromConfig(verifier.Options{
		CronAPIURL:   cfg.OpenClaw.APIURL,
		CronEndpoint: cfg.OpenClaw.CronEndpoint,
		BeadsCommand: cfg.Verification.BeadsCommand,
		StateDirs:    cfg.Verification.StateDirs,
		MemoryDirs:   cfg.Verification.MemoryDirs,
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
			return &grace.VerificationOutcome{IsBacked: false}, err
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
			log.Printf("failed to create bead for %s: %v", meta.CommitmentID, err)
			return
		}
		log.Printf("created bead %s for unbacked commitment %s", beadID, meta.CommitmentID)

		// Fire webhook notification
		if webhook != nil {
			if err := webhook.NotifyUnbacked(beadID, meta.Message, meta.Category); err != nil {
				log.Printf("webhook notification failed for %s: %v", beadID, err)
			}
		}
		if err := relayPublisher.NotifyUnbackedWithContext(beadID, meta.Message, meta.Category, meta.SessionKey, meta.CommitmentID); err != nil {
			log.Printf("relay notification failed for %s: %v", beadID, err)
		}
	})

	// Set the resolve callback to fire webhooks and relay notifications when beads are resolved.
	v2.SetResolveCallback(func(beadID, evidence string) {
		if dryRun {
			log.Printf("dry-run: would notify resolution for %s", beadID)
			return
		}
		if resolutionWebhook != nil {
			if err := resolutionWebhook.NotifyResolved(beadID, evidence); err != nil {
				log.Printf("resolve webhook failed for %s: %v", beadID, err)
			}
		}
		if err := relayPublisher.NotifyResolved(beadID, evidence); err != nil {
			log.Printf("resolve relay publish failed for %s: %v", beadID, err)
		}
	})

	addr := cfg.Server.Addr
	if addr == "" {
		addr = ":9876"
	}
	mux := http.NewServeMux()

	// Register v2 API routes
	v2Handler := v2.Handler()
	mux.Handle("/api/v2/", v2Handler)
	mux.Handle("/api/v2/analyze", v2Handler)
	mux.Handle("/api/v2/stats", v2Handler)
	mux.Handle("/api/v2/commitments", v2Handler)

	// Health endpoints
	healthHandler := api.NewHealthHandler()
	readyHandler := api.NewReadinessHandler(cfg.Verification.BeadsCommand)
	mux.Handle("/healthz", healthHandler)
	mux.Handle("/readyz", readyHandler)

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
				server.Shutdown(shutCtx)
				return nil
			}
		},
		OnStop: func() {
			gracePeriod.Stop()
			fmt.Println("Oathkeeper stopped.")
		},
	})

	if err := d.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
