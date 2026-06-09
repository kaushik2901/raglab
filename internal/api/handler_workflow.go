package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/riverqueue/river"
)

func (s *Server) preprocessHandler(w http.ResponseWriter, r *http.Request) {
	var req PreprocessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, 400, "INVALID_JSON", "invalid request body")
		return
	}
	if err := req.Validate(); err != nil {
		respondError(w, 400, "INVALID_PARAMETER", err.Error())
		return
	}
	resp, err := s.workflows.InsertPreprocess(r.Context(), req)
	if err != nil {
		respondError(w, 500, "INTERNAL_ERROR", err.Error())
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/v1/workflows/%d", resp.JobID))
	respondJSON(w, 202, resp)
}

func (s *Server) indexHandler(w http.ResponseWriter, r *http.Request) {
	var req IndexRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, 400, "INVALID_JSON", "invalid request body")
		return
	}
	if err := req.Validate(); err != nil {
		respondError(w, 400, "INVALID_PARAMETER", err.Error())
		return
	}
	resp, err := s.workflows.InsertIndex(r.Context(), req)
	if err != nil {
		respondError(w, 500, "INTERNAL_ERROR", err.Error())
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/v1/workflows/%d", resp.JobID))
	respondJSON(w, 202, resp)
}

func (s *Server) evalHandler(w http.ResponseWriter, r *http.Request) {
	var req EvalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, 400, "INVALID_JSON", "invalid request body")
		return
	}
	if err := req.Validate(); err != nil {
		respondError(w, 400, "INVALID_PARAMETER", err.Error())
		return
	}
	resp, err := s.workflows.InsertEval(r.Context(), req)
	if err != nil {
		respondError(w, 500, "INTERNAL_ERROR", err.Error())
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/v1/workflows/%d", resp.JobID))
	respondJSON(w, 202, resp)
}

func (s *Server) workflowStatusHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondError(w, 400, "INVALID_PARAMETER", "invalid job id")
		return
	}

	resp, err := s.workflows.GetJob(r.Context(), id)
	if err != nil {
		if errors.Is(err, river.ErrNotFound) {
			respondError(w, 404, "NOT_FOUND", "job not found")
			return
		}
		respondError(w, 500, "INTERNAL_ERROR", err.Error())
		return
	}
	respondJSON(w, 200, resp)
}
