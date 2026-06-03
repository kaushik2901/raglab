package chunker

import "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"

type Chunker interface {
	Chunk(doc types.Document) ([]types.Chunk, error)
}
