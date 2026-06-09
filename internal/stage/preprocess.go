package stage

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/preprocessor"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func PreprocessStage(outputPath string, includeDirs []string) types.Stage {
	return types.Stage{
		Name:     "preprocess",
		Requires: []types.StageID{"clone"},
		Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
			repoPath, ok := state["repo_path"].(string)
			if !ok || repoPath == "" {
				return nil, fmt.Errorf("repo_path not found in state")
			}
			srcDir := filepath.Join(repoPath, "content")
			dstDir := outputPath
			count, err := preprocessor.ProcessAllFiles(ctx, srcDir, includeDirs, dstDir, 10)
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
