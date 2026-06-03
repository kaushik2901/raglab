package stageimport

import (
	"context"
	"fmt"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/store"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func StoreStage(cfg *config.Config) types.Stage {
	return types.Stage{
		Name:     "store",
		Requires: []types.StageID{"embed"},
		Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
			docChunks, ok := state["document_chunks"].([]types.DocumentChunk)
			if !ok {
				return nil, fmt.Errorf("document_chunks not found in state")
			}

			if len(docChunks) == 0 {
				return &types.StageResult{
					Name:   "store",
					Output: map[string]any{"stored_count": 0},
				}, nil
			}

			vectorSize := docChunks[0].Embedding.Dimensions

			qStore := store.NewQdrantStore(cfg.QdrantAPIKey)
			if err := qStore.Connect(ctx, cfg.QdrantURL); err != nil {
				return nil, fmt.Errorf("connect: %w", err)
			}
			defer qStore.Close()

			if err := qStore.EnsureCollection(ctx, "document_chunks", vectorSize, "Cosine"); err != nil {
				return nil, fmt.Errorf("ensure collection: %w", err)
			}

			if err := qStore.Store(ctx, docChunks); err != nil {
				return nil, fmt.Errorf("store: %w", err)
			}

			return &types.StageResult{
				Name:   "store",
				Output: map[string]any{"stored_count": len(docChunks)},
			}, nil
		},
	}
}
