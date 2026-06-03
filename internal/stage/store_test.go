package stageimport

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func TestStoreStage_Basic(t *testing.T) {
	t.Skip("requires Qdrant server")
	state := map[string]any{
		"document_chunks": []types.DocumentChunk{
			{
				Chunk: types.Chunk{
					ID:           "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
					DocumentPath: "doc.md",
					Content:      "test",
					TokenCount:   1,
					Index:        0,
				},
				Embedding: types.Embedding{
					ChunkID:    "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
					Vector:     []float64{0.1, 0.2, 0.3},
					Model:      "test-model",
					Dimensions: 3,
				},
			},
		},
	}

	stage := StoreStage("http://localhost:6334", "")
	result, err := stage.Run(context.Background(), state)
	require.NoError(t, err)
	require.NoError(t, result.Err)

	count, ok := result.Output["stored_count"]
	require.True(t, ok)
	assert.Equal(t, 1, count)
}

func TestStoreStage_EmptyChunks(t *testing.T) {
	state := map[string]any{
		"document_chunks": []types.DocumentChunk{},
	}

	stage := StoreStage("http://localhost:6334", "")
	result, err := stage.Run(context.Background(), state)
	require.NoError(t, err)
	require.NoError(t, result.Err)

	count, ok := result.Output["stored_count"]
	require.True(t, ok)
	assert.Equal(t, 0, count)
}

func TestStoreStage_StateKey(t *testing.T) {
	state := map[string]any{
		"document_chunks": []types.DocumentChunk{
			{
				Chunk: types.Chunk{
					ID:           "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
					DocumentPath: "doc.md",
					Content:      "empty test key",
					TokenCount:   3,
					Index:        0,
				},
				Embedding: types.Embedding{
					ChunkID:    "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
					Vector:     []float64{0.5},
					Model:      "m",
					Dimensions: 1,
				},
			},
		},
	}
	_ = state
}

func TestStoreStage_CollectionName(t *testing.T) {
	t.Skip("requires Qdrant server")
	state := map[string]any{
		"document_chunks": []types.DocumentChunk{
			{
				Chunk: types.Chunk{
					ID:           "cccccccc-cccc-cccc-cccc-cccccccccccc",
					DocumentPath: "doc.md",
					Content:      "collection test",
					TokenCount:   2,
					Index:        0,
				},
				Embedding: types.Embedding{
					ChunkID:    "cccccccc-cccc-cccc-cccc-cccccccccccc",
					Vector:     []float64{0.1, 0.2},
					Model:      "m",
					Dimensions: 2,
				},
			},
		},
	}

	stage := StoreStage("http://localhost:6334", "")
	result, err := stage.Run(context.Background(), state)
	require.NoError(t, err)
	require.NoError(t, result.Err)

	count, ok := result.Output["stored_count"]
	require.True(t, ok)
	assert.Equal(t, 1, count)
}

func TestStoreStage_ConnectionError(t *testing.T) {
	t.Skip("requires Qdrant server to reliably test connection")
	state := map[string]any{
		"document_chunks": []types.DocumentChunk{
			{
				Chunk: types.Chunk{
					ID:           "dddddddd-dddd-dddd-dddd-dddddddddddd",
					DocumentPath: "doc.md",
					Content:      "connection error test",
					TokenCount:   3,
					Index:        0,
				},
				Embedding: types.Embedding{
					ChunkID:    "dddddddd-dddd-dddd-dddd-dddddddddddd",
					Vector:     []float64{0.1},
					Model:      "m",
					Dimensions: 1,
				},
			},
		},
	}

	stage := StoreStage("http://localhost:1", "")
	_, err := stage.Run(context.Background(), state)
	require.Error(t, err)
}
