package stage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreprocessStage_Execute(t *testing.T) {
	srcDir := t.TempDir()
	contentDir := filepath.Join(srcDir, "content")
	os.MkdirAll(contentDir, 0755)
	os.WriteFile(filepath.Join(contentDir, "page.md"), []byte("# Hello"), 0644)

	dstDir := t.TempDir()

	stage := PreprocessStage(dstDir, nil)
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

	stage := PreprocessStage(dstDir, nil)
	result, err := stage.Run(context.Background(), map[string]any{
		"repo_path": srcDir,
	})
	require.NoError(t, err)
	require.NoError(t, result.Err)
}

func TestPreprocessStage_Name(t *testing.T) {
	stage := PreprocessStage(t.TempDir(), nil)
	assert.Equal(t, "preprocess", string(stage.Name))
}

func TestPreprocessStage_Requires(t *testing.T) {
	stage := PreprocessStage(t.TempDir(), nil)
	assert.Equal(t, 1, len(stage.Requires))
	assert.Equal(t, "clone", string(stage.Requires[0]))
}
