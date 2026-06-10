package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

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

// New creates a Server by connecting to Postgres and Qdrant.
// This is the convenience entry point for cmd/api/main.go.
func New(cfg *config.Config) (*Server, error) {
	pool, err := db.Connect(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	qdrantStore := qstore.NewQdrantStore(cfg.QdrantAPIKey)
	if err := qdrantStore.Connect(context.Background(), cfg.QdrantURL); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect qdrant: %w", err)
	}

	return NewWithDeps(cfg, pool, qdrantStore), nil
}

// NewWithDeps creates a Server with explicitly provided connections.
// This is the testable version — no infrastructure creation.
func NewWithDeps(cfg *config.Config, pool *pgxpool.Pool, qdrant qstore.VectorStore) *Server {
	r := chi.NewRouter()
	r.Use(RequestID)
	r.Use(StructuredLog)
	r.Use(Recovery)
	r.Use(MaxBodySize(10 << 20))
	r.Use(Timeout(cfg.APIRequestTimeout))

	s := &Server{
		cfg:    cfg,
		router: r,
		pool:   pool,
		qdrant: qdrant,
	}

	NewHealthRouter(pool, qdrant).Register(r)
	NewArtifactRouter(cfg.ArtifactsDir).Register(r)

	evalSvc := NewEvalService(pool)
	r.Route("/api/v1/eval", func(r chi.Router) {
		NewEvalRouter(evalSvc).Register(r)
	})

	workflowSvc := s.newWorkflowService(pool)
	if workflowSvc != nil {
		r.Route("/api/v1/workflows", func(r chi.Router) {
			NewWorkflowRouter(workflowSvc).Register(r)
		})
	}

	chatSvc, _ := NewChatService(cfg, qdrant)
	r.Route("/api/v1/chat", func(r chi.Router) {
		NewChatRouter(chatSvc).Register(r)
	})

	r.Get("/", indexHandler)

	return s
}

func (s *Server) newWorkflowService(pool *pgxpool.Pool) *WorkflowService {
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		MaxAttempts: s.cfg.MaxRetries,
	})
	if err != nil {
		slog.Warn("river client init failed, workflow endpoints unavailable", "err", err)
		return nil
	}
	return NewWorkflowService(client)
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
