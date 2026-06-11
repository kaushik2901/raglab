package chunker

import (
	"context"
	"fmt"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type Chunker interface {
	Chunk(ctx context.Context, reader types.ElementReader, docPath string) (<-chan types.Chunk, <-chan error)
}

type ChunkerFactory func(cfg map[string]any) (Chunker, error)

var chunkers = map[string]ChunkerFactory{}

func New(strategy string, cfg map[string]any) (Chunker, error) {
	fn, ok := chunkers[strategy]
	if !ok {
		return nil, fmt.Errorf("unknown chunker %q", strategy)
	}
	if cfg == nil {
		return nil, fmt.Errorf("chunker config must not be nil")
	}
	return fn(cfg)
}

func RegisterChunker(name string, fn ChunkerFactory) {
	chunkers[name] = fn
}

func getInt(m map[string]any, key string, def int) int {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok {
		return def
	}
	f, ok := v.(float64)
	if ok {
		return int(f)
	}
	i, ok := v.(int)
	if ok {
		return i
	}
	return def
}

func getString(m map[string]any, key string, def string) string {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok {
		return def
	}
	s, ok := v.(string)
	if ok {
		return s
	}
	return def
}
