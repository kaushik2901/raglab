package chunker

import (
	"context"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func init() {
	RegisterChunker("semantic", func(size, overlap int) Chunker {
		return NewSemanticChunker(size, overlap)
	})
}

type SemanticChunker struct {
	Size    int
	Overlap int
}

func NewSemanticChunker(size, overlap int) *SemanticChunker {
	return &SemanticChunker{Size: size, Overlap: overlap}
}

func (c *SemanticChunker) Chunk(ctx context.Context, reader types.ElementReader, docPath string) (<-chan types.Chunk, <-chan error) {
	return NewFixedChunker(c.Size, c.Overlap).Chunk(ctx, reader, docPath)
}
