package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	qstore "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/store"
)

type HealthRouter struct {
	pool   *pgxpool.Pool
	qdrant qstore.VectorStore
}

func NewHealthRouter(pool *pgxpool.Pool, qdrant qstore.VectorStore) *HealthRouter {
	return &HealthRouter{pool: pool, qdrant: qdrant}
}

func (r *HealthRouter) Register(mux chi.Router) {
	mux.Get("/health", r.healthHandler)
}

func (r *HealthRouter) healthHandler(w http.ResponseWriter, req *http.Request) {
	services := map[string]string{}

	if r.pool == nil {
		services["postgres"] = "disconnected: not initialized"
	} else if err := r.pool.Ping(req.Context()); err != nil {
		services["postgres"] = "disconnected: " + err.Error()
	} else {
		services["postgres"] = "connected"
	}

	if err := r.qdrant.HealthCheck(req.Context()); err != nil {
		services["qdrant"] = "disconnected: " + err.Error()
	} else {
		services["qdrant"] = "connected"
	}

	allOK := true
	for _, status := range services {
		if status != "connected" {
			allOK = false
			break
		}
	}

	if !allOK {
		respondJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status":   "degraded",
			"version":  version,
			"services": services,
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"version":  version,
		"services": services,
	})
}
