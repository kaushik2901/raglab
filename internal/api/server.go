package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/db"
	qstore "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/store"
)

const version = "0.1.0"

type Server struct {
	cfg    *config.Config
	router *chi.Mux
	http   *http.Server
	pool   *pgxpool.Pool
	qdrant qstore.VectorStore
}

func New(cfg *config.Config) (*Server, error) {
	pool, err := db.Connect(context.Background())
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	qdrantStore := qstore.NewQdrantStore(cfg.QdrantAPIKey)
	if err := qdrantStore.Connect(context.Background(), cfg.QdrantURL); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect qdrant: %w", err)
	}

	r := chi.NewRouter()
	r.Use(RequestID)
	r.Use(StructuredLog)
	r.Use(Recovery)
	r.Use(Timeout(60 * time.Second))
	r.Use(CORS)

	s := &Server{
		cfg:    cfg,
		router: r,
		pool:   pool,
		qdrant: qdrantStore,
	}

	r.Get("/health", s.healthHandler)

	return s, nil
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	addr := fmt.Sprintf(":%s", config.EnvOrDefault("API_PORT", "8080"))
	s.http = &http.Server{Addr: addr, Handler: s.router}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.http.Shutdown(shutdownCtx)
	}()

	slog.Info("api server starting", "addr", addr)
	if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
