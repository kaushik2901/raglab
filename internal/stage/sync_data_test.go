package stageimport

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
)

func TestSyncDataStage_Execute(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	projectRoot := t.TempDir()
	scriptDir := filepath.Join(projectRoot, "handbook", "scripts")
	os.MkdirAll(scriptDir, 0755)
	scriptContent := `#!/bin/sh
set -eu
mkdir -p "data/public"
echo "sync complete"
`
	os.WriteFile(filepath.Join(scriptDir, "sync-data.sh"), []byte(scriptContent), 0755)

	repoPath := t.TempDir()

	cwd, _ := os.Getwd()
	os.Chdir(projectRoot)
	defer os.Chdir(cwd)

	cfg := &config.Config{}
	stage := SyncDataStage(cfg)

	result, err := stage.Run(context.Background(), map[string]any{
		"repo_path": repoPath,
	})
	require.NoError(t, err)
	require.NoError(t, result.Err)

	synced, ok := result.Output["synced"].(bool)
	assert.True(t, ok)
	assert.True(t, synced)

	dataDir := filepath.Join(repoPath, "data", "public")
	_, err = os.Stat(dataDir)
	assert.False(t, os.IsNotExist(err), "data/public directory should exist")
}

func TestSyncDataStage_Execute_MissingRepoPath(t *testing.T) {
	cfg := &config.Config{}
	stage := SyncDataStage(cfg)

	_, err := stage.Run(context.Background(), map[string]any{})
	assert.Error(t, err)
}

func TestSyncDataStage_Execute_EmptyRepoPath(t *testing.T) {
	cfg := &config.Config{}
	stage := SyncDataStage(cfg)

	_, err := stage.Run(context.Background(), map[string]any{
		"repo_path": "",
	})
	assert.Error(t, err)
}

func TestSyncDataStage_Name(t *testing.T) {
	cfg := &config.Config{}
	stage := SyncDataStage(cfg)
	assert.Equal(t, "sync-data", string(stage.Name))
}

func TestSyncDataStage_Requires(t *testing.T) {
	cfg := &config.Config{}
	stage := SyncDataStage(cfg)
	assert.Equal(t, 1, len(stage.Requires))
	assert.Equal(t, "clone", string(stage.Requires[0]))
}
