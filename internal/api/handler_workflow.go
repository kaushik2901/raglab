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
		respondProblem(w, 400, "Invalid Request Body", "invalid request body")
		return
	}
	if err := req.Validate(); err != nil {
		respondProblem(w, 400, "Invalid Parameter", err.Error())
		return
	}
	resp, err := s.workflows.InsertPreprocess(r.Context(), req)
	if err != nil {
		respondProblem(w, 500, "Internal Server Error", err.Error())
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/v1/workflows/%d", resp.JobID))
	respondJSON(w, 202, resp)
}

func (s *Server) indexHandler(w http.ResponseWriter, r *http.Request) {
	var req IndexRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondProblem(w, 400, "Invalid Request Body", "invalid request body")
		return
	}
	if err := req.Validate(); err != nil {
		respondProblem(w, 400, "Invalid Parameter", err.Error())
		return
	}
	resp, err := s.workflows.InsertIndex(r.Context(), req)
	if err != nil {
		respondProblem(w, 500, "Internal Server Error", err.Error())
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/v1/workflows/%d", resp.JobID))
	respondJSON(w, 202, resp)
}

func (s *Server) evalHandler(w http.ResponseWriter, r *http.Request) {
	var req EvalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondProblem(w, 400, "Invalid Request Body", "invalid request body")
		return
	}
	if err := req.Validate(); err != nil {
		respondProblem(w, 400, "Invalid Parameter", err.Error())
		return
	}
	resp, err := s.workflows.InsertEval(r.Context(), req)
	if err != nil {
		respondProblem(w, 500, "Internal Server Error", err.Error())
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/v1/workflows/%d", resp.JobID))
	respondJSON(w, 202, resp)
}

func (s *Server) workflowStatusHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondProblem(w, 400, "Invalid Parameter", "invalid job id")
		return
	}

	resp, err := s.workflows.GetJob(r.Context(), id)
	if err != nil {
		if errors.Is(err, river.ErrNotFound) {
			respondProblem(w, 404, "Not Found", "job not found")
			return
		}
		respondProblem(w, 500, "Internal Server Error", err.Error())
		return
	}
	respondJSON(w, 200, resp)
}
