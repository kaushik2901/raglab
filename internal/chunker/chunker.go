package chunker

import (
	"context"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type Chunker interface {
	Chunk(ctx context.Context, reader types.ElementReader, docPath string) (<-chan types.Chunk, <-chan error)
}
