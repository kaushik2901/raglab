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

func (r *WorkflowRouter) listHandler(w http.ResponseWriter, req *http.Request) {
	kind := req.URL.Query().Get("kind")
	state := req.URL.Query().Get("state")
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(req.URL.Query().Get("offset"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	jobs, total, err := r.svc.ListJobs(req.Context(), kind, state, limit, offset)
	if err != nil {
		respondProblem(w, 500, "Internal Server Error", err.Error())
		return
	}
	respondJSON(w, 200, map[string]any{"jobs": jobs, "total": total})
}

type WorkflowRouter struct {
	svc *WorkflowService
}

func NewWorkflowRouter(svc *WorkflowService) *WorkflowRouter {
	return &WorkflowRouter{svc: svc}
}

func (r *WorkflowRouter) Register(mux chi.Router) {
	mux.Get("/", r.listHandler)
	mux.Post("/preprocess", r.preprocessHandler)
	mux.Post("/index", r.indexHandler)
	mux.Post("/eval", r.evalHandler)
	mux.Get("/{id}", r.workflowStatusHandler)
}

func (r *WorkflowRouter) preprocessHandler(w http.ResponseWriter, req *http.Request) {
	var body PreprocessRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		respondProblem(w, 400, "Invalid Request Body", "invalid request body")
		return
	}
	if err := body.Validate(); err != nil {
		respondProblem(w, 400, "Invalid Parameter", err.Error())
		return
	}
	resp, err := r.svc.InsertPreprocess(req.Context(), body)
	if err != nil {
		respondProblem(w, 500, "Internal Server Error", err.Error())
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/v1/workflows/%d", resp.JobID))
	respondJSON(w, 202, resp)
}

func (r *WorkflowRouter) indexHandler(w http.ResponseWriter, req *http.Request) {
	var body IndexRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		respondProblem(w, 400, "Invalid Request Body", "invalid request body")
		return
	}
	if err := body.Validate(); err != nil {
		respondProblem(w, 400, "Invalid Parameter", err.Error())
		return
	}
	resp, err := r.svc.InsertIndex(req.Context(), body)
	if err != nil {
		respondProblem(w, 500, "Internal Server Error", err.Error())
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/v1/workflows/%d", resp.JobID))
	respondJSON(w, 202, resp)
}

func (r *WorkflowRouter) evalHandler(w http.ResponseWriter, req *http.Request) {
	var body EvalRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		respondProblem(w, 400, "Invalid Request Body", "invalid request body")
		return
	}
	if err := body.Validate(); err != nil {
		respondProblem(w, 400, "Invalid Parameter", err.Error())
		return
	}
	resp, err := r.svc.InsertEval(req.Context(), body)
	if err != nil {
		respondProblem(w, 500, "Internal Server Error", err.Error())
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/v1/workflows/%d", resp.JobID))
	respondJSON(w, 202, resp)
}

func (r *WorkflowRouter) workflowStatusHandler(w http.ResponseWriter, req *http.Request) {
	idStr := chi.URLParam(req, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondProblem(w, 400, "Invalid Parameter", "invalid job id")
		return
	}

	resp, err := r.svc.GetJob(req.Context(), id)
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
