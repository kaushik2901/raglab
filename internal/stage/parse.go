package stageimport

import (
	"context"
	"fmt"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/parser"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func ParseStage(cfg *config.Config) types.Stage {
	return types.Stage{
		Name:     "parse",
		Requires: nil,
		Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
			docs, err := parser.ParseDir(cfg.OutputPath)
			if err != nil {
				return nil, fmt.Errorf("parse: %w", err)
			}
			return &types.StageResult{
				Name: "parse",
				Output: map[string]any{
					"documents":      docs,
					"document_count": len(docs),
				},
			}, nil
		},
	}
}
