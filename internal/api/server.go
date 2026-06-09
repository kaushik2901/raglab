package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
)

const version = "0.1.0"

type Server struct {
	cfg    *config.Config
	router *chi.Mux
	http   *http.Server
}

func New(cfg *config.Config) (*Server, error) {
	r := chi.NewRouter()
	r.Use(RequestID)
	r.Use(StructuredLog)
	r.Use(Recovery)
	r.Use(Timeout(60 * time.Second))
	r.Use(CORS)
	return &Server{
		cfg:    cfg,
		router: r,
	}, nil
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	addr := fmt.Sprintf(":%s", config.EnvOrDefault("API_PORT", "8080"))
	s.http = &http.Server{Addr: addr, Handler: s.router}

	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.http.Shutdown(shutdown)
	}()

	slog.Info("api server starting", "addr", addr)
	if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
