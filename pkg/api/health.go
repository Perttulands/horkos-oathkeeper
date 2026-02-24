package api

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"time"
)

// HealthHandler serves GET /healthz — returns 200 if the process is alive.
type HealthHandler struct{}

// NewHealthHandler creates a HealthHandler.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// ServeHTTP handles /healthz requests.
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ReadinessHandler serves GET /readyz — returns 200 if br CLI is accessible.
type ReadinessHandler struct {
	command string
}

// NewReadinessHandler creates a ReadinessHandler that checks the given br command.
func NewReadinessHandler(command string) *ReadinessHandler {
	if command == "" {
		command = "br"
	}
	return &ReadinessHandler{command: command}
}

// ServeHTTP handles /readyz requests.
func (h *ReadinessHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, h.command, "version")
	if err := cmd.Run(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"status":"not ready","error":%q}`, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
