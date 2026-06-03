package stageimport

import (
	"context"
	"fmt"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/parser"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func ParseStage(outputPath string) types.Stage {
	return types.Stage{
		Name:     "parse",
		Requires: nil,
		Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
			docs, err := parser.ParseDir(outputPath)
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
