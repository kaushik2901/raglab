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

func TestVerifyStage_Execute(t *testing.T) {
	srcDir := t.TempDir()
	contentDir := filepath.Join(srcDir, "content")
	os.MkdirAll(contentDir, 0755)
	os.WriteFile(filepath.Join(contentDir, "good.md"), []byte("# Valid\n\nSome content."), 0644)

	dstDir := t.TempDir()
	os.WriteFile(filepath.Join(dstDir, "good.md"), []byte("# Valid\n\nSome content."), 0644)

	cfg := &config.Config{
		OutputPath: dstDir,
	}
	stage := VerifyStage(cfg)

	result, err := stage.Run(context.Background(), map[string]any{
		"repo_path": srcDir,
	})
	require.NoError(t, err)
	require.NoError(t, result.Err)

	reportPath := filepath.Join(dstDir, "_verification_report.json")
	_, err = os.Stat(reportPath)
	assert.False(t, os.IsNotExist(err), "verification report should exist")
}

func TestVerifyStage_EmptyDir(t *testing.T) {
	srcDir := t.TempDir()
	os.MkdirAll(filepath.Join(srcDir, "content"), 0755)
	dstDir := t.TempDir()

	cfg := &config.Config{
		OutputPath: dstDir,
	}
	stage := VerifyStage(cfg)

	result, err := stage.Run(context.Background(), map[string]any{
		"repo_path": srcDir,
	})
	require.NoError(t, err)
	require.NoError(t, result.Err)

	reportPath := filepath.Join(dstDir, "_verification_report.json")
	_, err = os.Stat(reportPath)
	assert.False(t, os.IsNotExist(err), "verification report should exist")
}

func TestVerifyStage_Name(t *testing.T) {
	cfg := &config.Config{OutputPath: t.TempDir()}
	stage := VerifyStage(cfg)
	assert.Equal(t, "verify", string(stage.Name))
}

func TestVerifyStage_Requires(t *testing.T) {
	cfg := &config.Config{OutputPath: t.TempDir()}
	stage := VerifyStage(cfg)
	assert.Equal(t, 2, len(stage.Requires))
	assert.Equal(t, "clone", string(stage.Requires[0]))
	assert.Equal(t, "preprocess", string(stage.Requires[1]))
}
