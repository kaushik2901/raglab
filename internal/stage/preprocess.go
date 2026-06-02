package stageimport

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/pipeline"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/preprocessor"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func PreprocessStage(cfg *config.Config) pipeline.Stage {
	return pipeline.Stage{
		Name:     "preprocess",
		Requires: []types.StageID{"clone"},
		Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
			repoPath, ok := state["repo_path"].(string)
			if !ok || repoPath == "" {
				return nil, fmt.Errorf("repo_path not found in state")
			}
			srcDir := filepath.Join(repoPath, "content")
			dstDir := cfg.OutputPath
			count, err := preprocessor.ProcessAllFiles(srcDir, dstDir, 10)
			if err != nil {
				return nil, fmt.Errorf("preprocess: %w", err)
			}
			return &types.StageResult{
				Name:   "preprocess",
				Output: map[string]any{"processed_count": count},
			}, nil
		},
	}
}
