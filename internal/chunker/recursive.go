package chunker

import (
	"context"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func init() {
	RegisterChunker("recursive", func(size, overlap int) Chunker {
		return NewRecursiveChunker(size, overlap)
	})
}

type RecursiveChunker struct {
	Size    int
	Overlap int
}

func NewRecursiveChunker(size, overlap int) *RecursiveChunker {
	return &RecursiveChunker{Size: size, Overlap: overlap}
}

func (c *RecursiveChunker) Chunk(ctx context.Context, reader types.ElementReader, docPath string) (<-chan types.Chunk, <-chan error) {
	return NewFixedChunker(c.Size, c.Overlap).Chunk(ctx, reader, docPath)
}
