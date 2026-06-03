package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseArgs_Kind(t *testing.T) {
	assert.Equal(t, "parse", ParseArgs{}.Kind())
}

func TestChunkArgs_Kind(t *testing.T) {
	assert.Equal(t, "chunk", ChunkArgs{}.Kind())
}

func TestEmbedArgs_Kind(t *testing.T) {
	assert.Equal(t, "embed", EmbedArgs{}.Kind())
}

func TestStoreArgs_Kind(t *testing.T) {
	assert.Equal(t, "store", StoreArgs{}.Kind())
}

func TestRunChunkStep_Success(t *testing.T) {
	args := ChunkArgs{
		WorkflowID:    "test",
		Tag:           "idx-test",
		InputTag:      "pre-test",
		ChunkStrategy: "fixed",
		ChunkSize:     512,
		ChunkOverlap:  64,
	}
	state := map[string]any{
		"documents": []types.Document{
			{Path: "test.md", Content: strings.Repeat("hello world ", 100)},
		},
	}

	result, err := RunChunkStep(context.Background(), args, state)
	require.NoError(t, err)
	assert.Equal(t, "chunk", string(result.Name))
	assert.Greater(t, result.Output["chunk_count"], 0)
}

func TestRunChunkStep_EmptyDocuments(t *testing.T) {
	args := ChunkArgs{
		WorkflowID:    "test",
		Tag:           "idx-test",
		InputTag:      "pre-test",
		ChunkStrategy: "fixed",
		ChunkSize:     512,
		ChunkOverlap:  64,
	}
	state := map[string]any{
		"documents": []types.Document{},
	}

	result, err := RunChunkStep(context.Background(), args, state)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Output["chunk_count"])
}

func TestRunChunkStep_MissingDocuments(t *testing.T) {
	args := ChunkArgs{
		WorkflowID:    "test",
		Tag:           "idx-test",
		InputTag:      "pre-test",
		ChunkStrategy: "fixed",
		ChunkSize:     512,
		ChunkOverlap:  64,
	}

	_, err := RunChunkStep(context.Background(), args, map[string]any{})
	require.Error(t, err)
}

func TestRunParseStep_InputDirMissing(t *testing.T) {
	args := ParseArgs{
		WorkflowID: "test",
		Tag:        "idx-test",
		InputTag:   "nonexistent",
	}
	_, err := RunParseStep(context.Background(), args, map[string]any{})
	require.Error(t, err)
}

func TestRunParseStep_Success(t *testing.T) {
	dir := t.TempDir()
	inputDir := filepath.Join(dir, "artifacts", "preprocessing", "pre-test", "output")
	require.NoError(t, os.MkdirAll(inputDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(inputDir, "test.md"), []byte("# Hello World"), 0644))

	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer os.Chdir(origDir)

	args := ParseArgs{
		WorkflowID: "test",
		Tag:        "idx-test",
		InputTag:   "pre-test",
	}
	result, err := RunParseStep(context.Background(), args, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "parse", string(result.Name))
	assert.Equal(t, 1, result.Output["document_count"])
}
