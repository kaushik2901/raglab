package chunker

import (
	"fmt"
	"strings"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type FixedChunker struct {
	Size    int
	Overlap int
}

func NewFixedChunker(size, overlap int) *FixedChunker {
	if overlap >= size {
		overlap = size - 1
	}
	return &FixedChunker{Size: size, Overlap: overlap}
}

func (c *FixedChunker) Chunk(doc types.Document) ([]types.Chunk, error) {
	words := strings.Fields(doc.Content)
	if len(words) == 0 {
		return nil, nil
	}

	step := c.Size - c.Overlap
	if step <= 0 {
		step = 1
	}

	var chunks []types.Chunk
	idx := 0
	for start := 0; start < len(words); start += step {
		end := start + c.Size
		if end > len(words) {
			end = len(words)
		}
		chunkWords := words[start:end]
		content := strings.Join(chunkWords, " ")

		chunks = append(chunks, types.Chunk{
			ID:           fmt.Sprintf("%s-chunk-%04d", doc.Path, idx),
			DocumentPath: doc.Path,
			Content:      content,
			TokenCount:   estimateTokens(content),
			Index:        idx,
		})
		idx++

		if end == len(words) {
			break
		}
	}

	return chunks, nil
}

func estimateTokens(text string) int {
	return len(text) / 4
}
