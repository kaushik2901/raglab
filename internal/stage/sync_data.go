package stageimport

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/pipeline"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func SyncDataStage(cfg *config.Config) pipeline.Stage {
	return pipeline.Stage{
		Name:     "sync-data",
		Requires: []types.StageID{"clone"},
		Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
			repoPath, ok := state["repo_path"].(string)
			if !ok || repoPath == "" {
				return nil, fmt.Errorf("repo_path not found in state")
			}

			scriptPath := filepath.Join("handbook", "scripts", "sync-data.sh")
			absScriptPath, err := filepath.Abs(scriptPath)
			if err != nil {
				return nil, fmt.Errorf("resolve script path: %w", err)
			}

			cmd := exec.CommandContext(ctx, "sh", absScriptPath)
			cmd.Dir = repoPath
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			if err := cmd.Run(); err != nil {
				return nil, fmt.Errorf("sync-data: %w", err)
			}

			return &types.StageResult{
				Name:   "sync-data",
				Output: map[string]any{"synced": true},
			}, nil
		},
	}
}
