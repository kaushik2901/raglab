package stageimport

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
)

func TestPreprocessStage_Execute(t *testing.T) {
	srcDir := t.TempDir()
	contentDir := filepath.Join(srcDir, "content")
	os.MkdirAll(contentDir, 0755)
	os.WriteFile(filepath.Join(contentDir, "page.md"), []byte("# Hello"), 0644)

	dstDir := t.TempDir()

	cfg := &config.Config{
		OutputPath: dstDir,
	}
	stage := PreprocessStage(cfg)

	result, err := stage.Run(context.Background(), map[string]any{
		"repo_path": srcDir,
	})
	require.NoError(t, err)
	require.NoError(t, result.Err)

	_, err = os.Stat(filepath.Join(dstDir, "page.md"))
	assert.False(t, os.IsNotExist(err), "page.md should exist in output")
}

func TestPreprocessStage_Execute_EmptyDir(t *testing.T) {
	srcDir := t.TempDir()
	os.MkdirAll(filepath.Join(srcDir, "content"), 0755)
	dstDir := t.TempDir()

	cfg := &config.Config{
		OutputPath: dstDir,
	}
	stage := PreprocessStage(cfg)

	result, err := stage.Run(context.Background(), map[string]any{
		"repo_path": srcDir,
	})
	require.NoError(t, err)
	require.NoError(t, result.Err)
}

func TestPreprocessStage_Name(t *testing.T) {
	cfg := &config.Config{OutputPath: t.TempDir()}
	stage := PreprocessStage(cfg)
	assert.Equal(t, "preprocess", string(stage.Name))
}

func TestPreprocessStage_Requires(t *testing.T) {
	cfg := &config.Config{OutputPath: t.TempDir()}
	stage := PreprocessStage(cfg)
	assert.Equal(t, 1, len(stage.Requires))
	assert.Equal(t, "clone", string(stage.Requires[0]))
}
