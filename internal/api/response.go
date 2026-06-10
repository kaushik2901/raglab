package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type ProblemDetail struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance,omitempty"`
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]any{"data": data}); err != nil {
		slog.Warn("failed to encode response", "err", err)
	}
}

func respondProblem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ProblemDetail{
		Type:   problemType(status, title),
		Title:  title,
		Status: status,
		Detail: detail,
	})
}

func respondNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

func problemType(status int, title string) string {
	switch status {
	case 400:
		return "/errors/bad-request"
	case 404:
		return "/errors/not-found"
	case 500:
		return "/errors/internal-server-error"
	default:
		return "/errors/unknown"
	}
}
