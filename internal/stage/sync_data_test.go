package stageimport

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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
	if err != nil {
		t.Fatalf("SyncDataStage.Run: %v", err)
	}
	if result.Err != nil {
		t.Fatalf("result.Err = %v", result.Err)
	}

	synced, ok := result.Output["synced"].(bool)
	if !ok || !synced {
		t.Errorf("Output[\"synced\"] = %v, want true", result.Output["synced"])
	}

	dataDir := filepath.Join(repoPath, "data", "public")
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Errorf("data/public directory not created at %s", dataDir)
	}
}

func TestSyncDataStage_Execute_MissingRepoPath(t *testing.T) {
	cfg := &config.Config{}
	stage := SyncDataStage(cfg)

	_, err := stage.Run(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing repo_path, got nil")
	}
}

func TestSyncDataStage_Execute_EmptyRepoPath(t *testing.T) {
	cfg := &config.Config{}
	stage := SyncDataStage(cfg)

	_, err := stage.Run(context.Background(), map[string]any{
		"repo_path": "",
	})
	if err == nil {
		t.Fatal("expected error for empty repo_path, got nil")
	}
}

func TestSyncDataStage_Name(t *testing.T) {
	cfg := &config.Config{}
	stage := SyncDataStage(cfg)
	if stage.Name != "sync-data" {
		t.Errorf("got %q, want %q", stage.Name, "sync-data")
	}
}

func TestSyncDataStage_Requires(t *testing.T) {
	cfg := &config.Config{}
	stage := SyncDataStage(cfg)
	if len(stage.Requires) != 1 || stage.Requires[0] != "clone" {
		t.Errorf("Requires = %v, want [clone]", stage.Requires)
	}
}
