package stageimport

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func chunkWords(n int) string {
	ws := make([]string, n)
	for i := range ws {
		ws[i] = "word"
	}
	return strings.Join(ws, " ")
}

func TestChunkStage_Basic(t *testing.T) {
	state := map[string]any{
		"documents": []types.Document{
			{Path: "doc.md", Content: chunkWords(100)},
		},
	}

	stage := ChunkStage("fixed", 30, 10)
	result, err := stage.Run(context.Background(), state)
	require.NoError(t, err)
	require.NoError(t, result.Err)

	chunks, ok := result.Output["chunks"].([]types.Chunk)
	require.True(t, ok)
	assert.NotEmpty(t, chunks)
}

func TestChunkStage_StrategySelection(t *testing.T) {
	state := map[string]any{
		"documents": []types.Document{
			{Path: "doc.md", Content: chunkWords(10)},
		},
	}

	stage := ChunkStage("fixed", 10, 0)
	result, err := stage.Run(context.Background(), state)
	require.NoError(t, err)
	require.NoError(t, result.Err)

	chunks, ok := result.Output["chunks"].([]types.Chunk)
	require.True(t, ok)
	assert.Len(t, chunks, 1)
}

func TestChunkStage_StateKey(t *testing.T) {
	state := map[string]any{
		"documents": []types.Document{
			{Path: "a.md", Content: chunkWords(5)},
		},
	}

	stage := ChunkStage("fixed", 10, 0)
	result, err := stage.Run(context.Background(), state)
	require.NoError(t, err)
	require.NoError(t, result.Err)

	chunks, ok := result.Output["chunks"].([]types.Chunk)
	require.True(t, ok)
	assert.Len(t, chunks, 1)
	assert.Equal(t, "a.md", chunks[0].DocumentPath)
}

func TestChunkStage_ChunkCount(t *testing.T) {
	state := map[string]any{
		"documents": []types.Document{
			{Path: "a.md", Content: chunkWords(100)},
		},
	}

	stage := ChunkStage("fixed", 30, 0)
	result, err := stage.Run(context.Background(), state)
	require.NoError(t, err)
	require.NoError(t, result.Err)

	chunks, _ := result.Output["chunks"].([]types.Chunk)
	count, ok := result.Output["chunk_count"]
	require.True(t, ok)
	assert.Equal(t, len(chunks), count)
}

func TestChunkStage_EmptyDocuments(t *testing.T) {
	state := map[string]any{
		"documents": []types.Document{},
	}

	stage := ChunkStage("fixed", 10, 0)
	result, err := stage.Run(context.Background(), state)
	require.NoError(t, err)
	require.NoError(t, result.Err)

	chunks, _ := result.Output["chunks"].([]types.Chunk)
	assert.Empty(t, chunks)

	count, ok := result.Output["chunk_count"]
	require.True(t, ok)
	assert.Equal(t, 0, count)
}

func TestChunkStage_InvalidStrategy(t *testing.T) {
	state := map[string]any{
		"documents": []types.Document{
			{Path: "doc.md", Content: "hello"},
		},
	}

	stage := ChunkStage("invalid", 10, 0)
	_, err := stage.Run(context.Background(), state)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown chunk strategy")
}
