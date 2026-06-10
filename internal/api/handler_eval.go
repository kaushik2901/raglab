package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func (s *Server) evalListHandler(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	runs, total, err := s.evalSvc.ListRuns(r.Context(), limit, offset)
	if err != nil {
		respondProblem(w, 500, "Internal Server Error", err.Error())
		return
	}
	respondJSON(w, 200, map[string]any{"runs": runs, "total": total})
}

func (s *Server) evalDetailHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	run, err := s.evalSvc.GetRun(r.Context(), id, limit, offset)
	if err != nil {
		respondProblem(w, 404, "Not Found", "eval run not found")
		return
	}
	respondJSON(w, 200, run)
}

func (s *Server) evalCompareHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	compareIDs := r.URL.Query()["compare_to"]
	allIDs := append([]string{id}, compareIDs...)

	runs := make(map[string]RunSummary)
	for _, rid := range allIDs {
		detail, err := s.evalSvc.GetRun(r.Context(), rid, 0, 0)
		if err != nil {
			respondProblem(w, 404, "Not Found", fmt.Sprintf("run %s not found", rid))
			return
		}
		runs[rid] = detail.RunSummary
	}
	respondJSON(w, 200, map[string]any{"runs": runs})
}
