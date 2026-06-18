package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"

	qstore "github.com/kaushik2901/raglab/internal/store"
	"github.com/kaushik2901/raglab/internal/types"
)

type mockIndexStore struct {
	listCollectionsFn func(ctx context.Context) ([]qstore.CollectionInfo, error)
	getCollectionFn   func(ctx context.Context, name string) (*qstore.CollectionInfo, error)
	deleteCollectionFn func(ctx context.Context, name string) error
}

func (m *mockIndexStore) Connect(ctx context.Context, dsn string) error { return nil }
func (m *mockIndexStore) EnsureCollection(ctx context.Context, name string, vectorSize int, distance string) error {
	return nil
}
func (m *mockIndexStore) Store(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error {
	return nil
}
func (m *mockIndexStore) Search(ctx context.Context, collectionName string, queryVector []float32, topK int) ([]types.SearchResult, error) {
	return nil, nil
}
func (m *mockIndexStore) ListCollections(ctx context.Context) ([]qstore.CollectionInfo, error) {
	if m.listCollectionsFn != nil {
		return m.listCollectionsFn(ctx)
	}
	return nil, nil
}
func (m *mockIndexStore) GetCollection(ctx context.Context, name string) (*qstore.CollectionInfo, error) {
	if m.getCollectionFn != nil {
		return m.getCollectionFn(ctx, name)
	}
	return nil, nil
}
func (m *mockIndexStore) DeleteCollection(ctx context.Context, name string) error {
	if m.deleteCollectionFn != nil {
		return m.deleteCollectionFn(ctx, name)
	}
	return nil
}
func (m *mockIndexStore) HealthCheck(ctx context.Context) error { return nil }
func (m *mockIndexStore) Close() error                          { return nil }

func TestIndexRouter_List(t *testing.T) {
	r := &IndexRouter{
		store: &mockIndexStore{
			listCollectionsFn: func(ctx context.Context) ([]qstore.CollectionInfo, error) {
				return []qstore.CollectionInfo{
					{Name: "alpha", VectorSize: 768, Distance: "Cosine"},
					{Name: "beta", VectorSize: 1536, Distance: "Dot"},
				}, nil
			},
		},
	}

	mux := chi.NewRouter()
	r.Register(mux)

	req := httptest.NewRequest("GET", "/indexes", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].([]any)
	assert.Len(t, data, 2)
}

func TestIndexRouter_List_Empty(t *testing.T) {
	r := &IndexRouter{
		store: &mockIndexStore{
			listCollectionsFn: func(ctx context.Context) ([]qstore.CollectionInfo, error) {
				return []qstore.CollectionInfo{}, nil
			},
		},
	}

	mux := chi.NewRouter()
	r.Register(mux)

	req := httptest.NewRequest("GET", "/indexes", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].([]any)
	assert.Len(t, data, 0)
}

func TestIndexRouter_Get_Found(t *testing.T) {
	r := &IndexRouter{
		store: &mockIndexStore{
			getCollectionFn: func(ctx context.Context, name string) (*qstore.CollectionInfo, error) {
				return &qstore.CollectionInfo{Name: name, VectorSize: 768, Distance: "Cosine"}, nil
			},
		},
	}

	mux := chi.NewRouter()
	r.Register(mux)

	req := httptest.NewRequest("GET", "/indexes/my-collection", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]any)
	assert.Equal(t, "my-collection", data["name"])
}

func TestIndexRouter_Get_NotFound(t *testing.T) {
	r := &IndexRouter{
		store: &mockIndexStore{
			getCollectionFn: func(ctx context.Context, name string) (*qstore.CollectionInfo, error) {
				return nil, fmt.Errorf("%w: %s", qstore.ErrCollectionNotFound, name)
			},
		},
	}

	mux := chi.NewRouter()
	r.Register(mux)

	req := httptest.NewRequest("GET", "/indexes/non-existent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, 404, rec.Code)
}

func TestIndexRouter_Delete_Success(t *testing.T) {
	r := &IndexRouter{
		store: &mockIndexStore{
			deleteCollectionFn: func(ctx context.Context, name string) error {
				return nil
			},
		},
	}

	mux := chi.NewRouter()
	r.Register(mux)

	req := httptest.NewRequest("DELETE", "/indexes/my-collection", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]any)
	assert.Equal(t, "my-collection", data["deleted"])
}

func TestIndexRouter_Delete_NotFound(t *testing.T) {
	r := &IndexRouter{
		store: &mockIndexStore{
			deleteCollectionFn: func(ctx context.Context, name string) error {
				return qstore.ErrCollectionNotFound
			},
		},
	}

	mux := chi.NewRouter()
	r.Register(mux)

	req := httptest.NewRequest("DELETE", "/indexes/non-existent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, 404, rec.Code)
}
