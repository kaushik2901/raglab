package chunker

import (
	"context"
	"fmt"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type Chunker interface {
	Chunk(ctx context.Context, reader types.ElementReader, docPath string) (<-chan types.Chunk, <-chan error)
}

var chunkers = map[string]func(size, overlap int) Chunker{
	"fixed": func(size, overlap int) Chunker { return NewFixedChunker(size, overlap) },
}

func New(strategy string, size, overlap int) (Chunker, error) {
	fn, ok := chunkers[strategy]
	if !ok {
		return nil, fmt.Errorf("unknown chunker %q", strategy)
	}
	return fn(size, overlap), nil
}

func RegisterChunker(name string, fn func(size, overlap int) Chunker) {
	chunkers[name] = fn
}
