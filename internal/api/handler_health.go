package api

import (
	"net/http"
)

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	services := map[string]string{}

	if s.pool == nil {
		services["postgres"] = "disconnected: not initialized"
	} else if err := s.pool.Ping(r.Context()); err != nil {
		services["postgres"] = "disconnected: " + err.Error()
	} else {
		services["postgres"] = "connected"
	}

	if err := s.qdrant.HealthCheck(r.Context()); err != nil {
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
