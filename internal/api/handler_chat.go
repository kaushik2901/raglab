package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) chatHandler(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, 400, "INVALID_JSON", "invalid request body")
		return
	}
	if req.Query == "" {
		respondError(w, 400, "INVALID_PARAMETER", "query is required")
		return
	}
	if req.Tag == "" {
		respondError(w, 400, "INVALID_PARAMETER", "tag is required")
		return
	}

	resp, err := s.chat.Chat(r.Context(), req)
	if err != nil {
		respondError(w, 500, "INTERNAL_ERROR", err.Error())
		return
	}
	respondJSON(w, 200, resp)
}
