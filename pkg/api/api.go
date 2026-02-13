package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/perttulands/oathkeeper/pkg/storage"
)

const defaultAddr = ":9876"

// ListResponse is the JSON envelope for commitment list queries.
type ListResponse struct {
	Count       int                  `json:"count"`
	Commitments []storage.Commitment `json:"commitments"`
}

// ErrorResponse is the JSON envelope for error responses.
type ErrorResponse struct {
	Error string `json:"error"`
}

// HealthResponse is the JSON envelope for the health endpoint.
type HealthResponse struct {
	Status string `json:"status"`
	Time   string `json:"time"`
}

// Server exposes commitment data over HTTP (TCP or Unix socket).
type Server struct {
	store  *storage.Store
	addr   string
	server *http.Server
	sockPath string
}

// NewServer creates a Server that reads from the given store.
// Addr can be "host:port" for TCP or "unix:/path/to/socket" for Unix domain socket.
// Empty addr defaults to ":9876".
func NewServer(store *storage.Store, addr string) *Server {
	if addr == "" {
		addr = defaultAddr
	}
	return &Server{store: store, addr: addr}
}

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	mux.HandleFunc("/api/v1/commitments/", s.handleCommitmentByID)
	mux.HandleFunc("/api/v1/commitments", s.handleCommitments)
	return mux
}

// ListenAndServe starts the HTTP server. It blocks until Shutdown is called.
func (s *Server) ListenAndServe() error {
	s.server = &http.Server{Handler: s.handler()}

	var ln net.Listener
	var err error

	if strings.HasPrefix(s.addr, "unix:") {
		s.sockPath = s.addr[5:]
		os.Remove(s.sockPath) // clean up stale socket
		ln, err = net.Listen("unix", s.sockPath)
	} else {
		ln, err = net.Listen("tcp", s.addr)
	}
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	err = s.server.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown() {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.server.Shutdown(ctx)
	}
	if s.sockPath != "" {
		os.Remove(s.sockPath)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, HealthResponse{
		Status: "ok",
		Time:   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleCommitments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	filter := storage.ListFilter{
		Status:   r.URL.Query().Get("status"),
		Category: r.URL.Query().Get("category"),
	}

	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		d, err := time.ParseDuration(sinceStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid since parameter: %v", err))
			return
		}
		filter.Since = &d
	}

	commitments, err := s.store.List(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("list commitments: %v", err))
		return
	}

	if commitments == nil {
		commitments = []storage.Commitment{}
	}

	writeJSON(w, http.StatusOK, ListResponse{
		Count:       len(commitments),
		Commitments: commitments,
	})
}

func (s *Server) handleCommitmentByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/v1/commitments/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing commitment ID")
		return
	}

	c, err := s.store.Get(id)
	if err != nil {
		if err == storage.ErrNotFound {
			writeError(w, http.StatusNotFound, "commitment not found")
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("get commitment: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, c)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}

// unixDialer returns a DialContext function for connecting to a Unix domain socket.
func unixDialer(sockPath string) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", sockPath)
	}
}
