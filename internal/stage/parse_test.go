package stageimport

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func TestParseStage_Basic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "doc1.md"), []byte("# Hello"), 0644)
	os.WriteFile(filepath.Join(dir, "doc2.md"), []byte("# World"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "nested.md"), []byte("# Nested"), 0644)

	stage := ParseStage(dir)
	result, err := stage.Run(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NoError(t, result.Err)

	count, ok := result.Output["document_count"]
	require.True(t, ok)
	assert.Equal(t, 3, count)
}

func TestParseStage_StateKey(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# Content"), 0644)

	stage := ParseStage(dir)
	result, err := stage.Run(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NoError(t, result.Err)

	docs, ok := result.Output["documents"].([]types.Document)
	require.True(t, ok)
	require.Len(t, docs, 1)
	assert.Equal(t, "doc.md", docs[0].Path)
	assert.Equal(t, "# Content", docs[0].Content)
}

func TestParseStage_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	stage := ParseStage(dir)
	result, err := stage.Run(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NoError(t, result.Err)

	count, ok := result.Output["document_count"]
	require.True(t, ok)
	assert.Equal(t, 0, count)

	docs, ok := result.Output["documents"].([]types.Document)
	require.True(t, ok)
	assert.Empty(t, docs)
}

func TestParseStage_Error(t *testing.T) {
	stage := ParseStage(filepath.Join(t.TempDir(), "nonexistent"))
	_, err := stage.Run(context.Background(), map[string]any{})
	require.Error(t, err)
}
