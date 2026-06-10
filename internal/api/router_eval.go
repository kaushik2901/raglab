package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type EvalRouter struct {
	svc *EvalService
}

func NewEvalRouter(svc *EvalService) *EvalRouter {
	return &EvalRouter{svc: svc}
}

func (r *EvalRouter) Register(mux chi.Router) {
	mux.Get("/runs", r.listHandler)
	mux.Get("/runs/{id}", r.detailHandler)
	mux.Get("/runs/{id}/compare", r.compareHandler)
}

func (r *EvalRouter) listHandler(w http.ResponseWriter, req *http.Request) {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(req.URL.Query().Get("offset"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	runs, total, err := r.svc.ListRuns(req.Context(), limit, offset)
	if err != nil {
		respondProblem(w, 500, "Internal Server Error", err.Error())
		return
	}
	respondJSON(w, 200, map[string]any{"runs": runs, "total": total})
}

func (r *EvalRouter) detailHandler(w http.ResponseWriter, req *http.Request) {
	id := chi.URLParam(req, "id")
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(req.URL.Query().Get("offset"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	run, err := r.svc.GetRun(req.Context(), id, limit, offset)
	if err != nil {
		respondProblem(w, 404, "Not Found", "eval run not found")
		return
	}
	respondJSON(w, 200, run)
}

func (r *EvalRouter) compareHandler(w http.ResponseWriter, req *http.Request) {
	id := chi.URLParam(req, "id")
	compareIDs := req.URL.Query()["compare_to"]
	allIDs := append([]string{id}, compareIDs...)

	runs := make(map[string]RunSummary)
	for _, rid := range allIDs {
		detail, err := r.svc.GetRun(req.Context(), rid, 0, 0)
		if err != nil {
			respondProblem(w, 404, "Not Found", fmt.Sprintf("run %s not found", rid))
			return
		}
		runs[rid] = detail.RunSummary
	}
	respondJSON(w, 200, map[string]any{"runs": runs})
}
