package stageimport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func TestEmbedStage_Basic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"index": 0, "embedding": []float64{0.1, 0.2, 0.3}},
				{"index": 1, "embedding": []float64{0.4, 0.5, 0.6}},
			},
			"model": "test-model",
		})
	}))
	defer srv.Close()

	state := map[string]any{
		"chunks": []types.Chunk{
			{ID: "chunk-0000", Content: "hello", DocumentPath: "doc.md", Index: 0},
			{ID: "chunk-0001", Content: "world", DocumentPath: "doc.md", Index: 1},
		},
	}

	cfg := &config.Config{
		LLMBaseURL:   srv.URL,
		EmbeddingModel: "test-model",
		BatchSize:    10,
	}
	stage := EmbedStage(cfg)
	result, err := stage.Run(context.Background(), state)
	require.NoError(t, err)
	require.NoError(t, result.Err)

	docChunks, ok := result.Output["document_chunks"].([]types.DocumentChunk)
	require.True(t, ok)
	assert.Len(t, docChunks, 2)

	count, ok := result.Output["embedding_count"]
	require.True(t, ok)
	assert.Equal(t, 2, count)
}

func TestEmbedStage_StateKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data":  []map[string]any{{"index": 0, "embedding": []float64{0.1}}},
			"model": "m",
		})
	}))
	defer srv.Close()

	state := map[string]any{
		"chunks": []types.Chunk{
			{ID: "chunk-0000", Content: "hello"},
		},
	}

	cfg := &config.Config{
		LLMBaseURL:    srv.URL,
		EmbeddingModel: "m",
		BatchSize:     10,
	}
	stage := EmbedStage(cfg)
	result, err := stage.Run(context.Background(), state)
	require.NoError(t, err)
	require.NoError(t, result.Err)

	docChunks, ok := result.Output["document_chunks"].([]types.DocumentChunk)
	require.True(t, ok)
	require.Len(t, docChunks, 1)
	assert.Equal(t, "chunk-0000", docChunks[0].Chunk.ID)
	assert.Len(t, docChunks[0].Embedding.Vector, 1)
}

func TestEmbedStage_Count(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"index": 0, "embedding": []float64{0.1}},
				{"index": 1, "embedding": []float64{0.2}},
				{"index": 2, "embedding": []float64{0.3}},
			},
			"model": "m",
		})
	}))
	defer srv.Close()

	state := map[string]any{
		"chunks": []types.Chunk{
			{ID: "c1", Content: "a"},
			{ID: "c2", Content: "b"},
			{ID: "c3", Content: "c"},
		},
	}

	cfg := &config.Config{
		LLMBaseURL:    srv.URL,
		EmbeddingModel: "m",
		BatchSize:     10,
	}
	stage := EmbedStage(cfg)
	result, err := stage.Run(context.Background(), state)
	require.NoError(t, err)
	require.NoError(t, result.Err)

	count, ok := result.Output["embedding_count"]
	require.True(t, ok)
	assert.Equal(t, 3, count)
}

func TestEmbedStage_EmptyChunks(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	state := map[string]any{
		"chunks": []types.Chunk{},
	}

	cfg := &config.Config{
		LLMBaseURL:    srv.URL,
		EmbeddingModel: "m",
		BatchSize:     10,
	}
	stage := EmbedStage(cfg)
	result, err := stage.Run(context.Background(), state)
	require.NoError(t, err)
	require.NoError(t, result.Err)

	assert.False(t, called, "API should not be called for empty chunks")

	docChunks, _ := result.Output["document_chunks"].([]types.DocumentChunk)
	assert.Empty(t, docChunks)
}

func TestEmbedStage_ChunkEmbeddingPairing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"index": 0, "embedding": []float64{0.1}},
				{"index": 1, "embedding": []float64{0.2}},
			},
			"model": "m",
		})
	}))
	defer srv.Close()

	state := map[string]any{
		"chunks": []types.Chunk{
			{ID: "my-id-a", Content: "first"},
			{ID: "my-id-b", Content: "second"},
		},
	}

	cfg := &config.Config{
		LLMBaseURL:    srv.URL,
		EmbeddingModel: "m",
		BatchSize:     10,
	}
	stage := EmbedStage(cfg)
	result, err := stage.Run(context.Background(), state)
	require.NoError(t, err)
	require.NoError(t, result.Err)

	docChunks, ok := result.Output["document_chunks"].([]types.DocumentChunk)
	require.True(t, ok)
	require.Len(t, docChunks, 2)

	assert.Equal(t, "my-id-a", docChunks[0].Chunk.ID)
	assert.Equal(t, "my-id-a", docChunks[0].Embedding.ChunkID)
	assert.Equal(t, "first", docChunks[0].Chunk.Content)

	assert.Equal(t, "my-id-b", docChunks[1].Chunk.ID)
	assert.Equal(t, "my-id-b", docChunks[1].Embedding.ChunkID)
	assert.Equal(t, "second", docChunks[1].Chunk.Content)
}
