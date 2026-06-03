package stageimport

import (
	"context"
	"fmt"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/chunker"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func ChunkStage(cfg *config.Config) types.Stage {
	return types.Stage{
		Name:     "chunk",
		Requires: []types.StageID{"parse"},
		Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
			docs, ok := state["documents"].([]types.Document)
			if !ok {
				return nil, fmt.Errorf("documents not found in state")
			}

			var ch chunker.Chunker
			switch cfg.ChunkStrategy {
			case "fixed":
				ch = chunker.NewFixedChunker(cfg.ChunkSize, cfg.ChunkOverlap)
			default:
				return nil, fmt.Errorf("unknown chunk strategy: %s", cfg.ChunkStrategy)
			}

			var allChunks []types.Chunk
			for _, doc := range docs {
				chunks, err := ch.Chunk(doc)
				if err != nil {
					return nil, fmt.Errorf("chunk %s: %w", doc.Path, err)
				}
				allChunks = append(allChunks, chunks...)
			}

			return &types.StageResult{
				Name: "chunk",
				Output: map[string]any{
					"chunks":      allChunks,
					"chunk_count": len(allChunks),
				},
			}, nil
		},
	}
}
