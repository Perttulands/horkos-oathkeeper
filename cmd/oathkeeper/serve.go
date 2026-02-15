package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/perttulands/oathkeeper/pkg/api"
	"github.com/perttulands/oathkeeper/pkg/beads"
	"github.com/perttulands/oathkeeper/pkg/daemon"
	"github.com/perttulands/oathkeeper/pkg/detector"
	"github.com/perttulands/oathkeeper/pkg/grace"
	"github.com/perttulands/oathkeeper/pkg/verifier"
)

func startServer(configPath string) {
	cfg := loadConfig(configPath)

	// Wire dependencies
	beadStore := beads.NewBeadStore(cfg.Verification.BeadsCommand)
	det := detector.NewDetector()
	ver := verifier.NewVerifier(cfg.OpenClaw.APIURL)

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

	addr := ":9876"
	mux := http.NewServeMux()

	// Register v2 API routes
	v2Handler := v2.Handler()
	mux.Handle("/api/v2/", v2Handler)
	mux.Handle("/api/v2/analyze", v2Handler)
	mux.Handle("/api/v2/stats", v2Handler)
	mux.Handle("/api/v2/commitments", v2Handler)

	// Health endpoints
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok"}`)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := beadStore.List(beads.Filter{Status: "open"})
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"not ready","error":%q}`, err.Error())
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ready"}`)
	})

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
