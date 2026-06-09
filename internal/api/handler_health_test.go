package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type mockQdrant struct {
	healthFn func(ctx context.Context) error
}

func (m *mockQdrant) Connect(ctx context.Context, dsn string) error { return nil }
func (m *mockQdrant) EnsureCollection(ctx context.Context, name string, vectorSize int, distance string) error {
	return nil
}
func (m *mockQdrant) Store(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error { return nil }
func (m *mockQdrant) Search(ctx context.Context, collectionName string, queryVector []float32, topK int) ([]types.SearchResult, error) {
	return nil, nil
}
func (m *mockQdrant) HealthCheck(ctx context.Context) error { return m.healthFn(ctx) }
func (m *mockQdrant) Close() error                         { return nil }

func TestHealthHandler_NilPool(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/health", nil)

	s := &Server{
		pool: nil,
		qdrant: &mockQdrant{
			healthFn: func(ctx context.Context) error { return errors.New("connection refused") },
		},
	}

	s.healthHandler(w, r)

	assert.Equal(t, 503, w.Code)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	data := resp["data"].(map[string]any)
	assert.Equal(t, "degraded", data["status"])
	services := data["services"].(map[string]any)
	assert.Equal(t, "disconnected: not initialized", services["postgres"])
	assert.Contains(t, services["qdrant"].(string), "disconnected")
}

func TestHealthHandler_BothDown(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/health", nil)

	s := &Server{
		pool: nil,
		qdrant: &mockQdrant{
			healthFn: func(ctx context.Context) error { return errors.New("down") },
		},
	}

	s.healthHandler(w, r)

	assert.Equal(t, 503, w.Code)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	data := resp["data"].(map[string]any)
	assert.Equal(t, "degraded", data["status"])
}

func TestHealthHandler_QdrantOK(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/health", nil)

	s := &Server{
		pool: nil,
		qdrant: &mockQdrant{
			healthFn: func(ctx context.Context) error { return nil },
		},
	}

	s.healthHandler(w, r)

	assert.Equal(t, 503, w.Code)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	data := resp["data"].(map[string]any)
	assert.Equal(t, "degraded", data["status"])
	services := data["services"].(map[string]any)
	assert.Equal(t, "connected", services["qdrant"])
}
