package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// ErrorResponse is the JSON envelope for API error responses.
type ErrorResponse struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"`
	Hint   string `json:"hint,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: encode failed: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeErrorWithDetail(w, status, msg, "", "")
}

func writeErrorWithDetail(w http.ResponseWriter, status int, msg string, detail string, hint string) {
	writeJSON(w, status, ErrorResponse{
		Error:  msg,
		Detail: strings.TrimSpace(detail),
		Hint:   strings.TrimSpace(hint),
	})
}
