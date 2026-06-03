package stageimport

import (
	"context"
	"fmt"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/embedder"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/pipeline"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func EmbedStage(cfg *config.Config) pipeline.Stage {
	return pipeline.Stage{
		Name:     "embed",
		Requires: []types.StageID{"chunk"},
		Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
			chunks, ok := state["chunks"].([]types.Chunk)
			if !ok {
				return nil, fmt.Errorf("chunks not found in state")
			}

			e := embedder.New(cfg.LLMBaseURL, cfg.LLMApiKey, cfg.EmbeddingModel, cfg.BatchSize)
			embeddings, err := e.Embed(ctx, chunks)
			if err != nil {
				return nil, fmt.Errorf("embed: %w", err)
			}

			docChunks := make([]types.DocumentChunk, len(chunks))
			for i := range chunks {
				docChunks[i] = types.DocumentChunk{
					Chunk:     chunks[i],
					Embedding: embeddings[i],
				}
			}

			return &types.StageResult{
				Name: "embed",
				Output: map[string]any{
					"document_chunks": docChunks,
					"embedding_count": len(docChunks),
				},
			}, nil
		},
	}
}
