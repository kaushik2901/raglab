package chunker

import (
	"context"
	"fmt"
	"strings"

	"github.com/kaushik2901/raglab/internal/types"
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

func (c *FixedChunker) Chunk(ctx context.Context, reader types.ElementReader, docPath string) (<-chan types.Chunk, <-chan error) {
	chunkCh := make(chan types.Chunk)
	errCh := make(chan error, 1)

	go func() {
		defer close(chunkCh)
		defer close(errCh)

		var window []string
		step := c.Size - c.Overlap
		if step <= 0 {
			step = 1
		}
		idx := 0

		for {
			elem, err := reader.ReadElement()
			if err != nil {
				if err.Error() == "EOF" {
					break
				}
				errCh <- fmt.Errorf("read element: %w", err)
				return
			}

			words := strings.Fields(elem.Text)
			for _, word := range words {
				select {
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				default:
				}

				window = append(window, word)

				for len(window) >= c.Size {
					content := strings.Join(window[:c.Size], " ")
				chunk := types.Chunk{
					ID:           fmt.Sprintf("%s-chunk-%04d", docPath, idx),
					DocumentPath: docPath,
					Content:      content,
					Metadata:     reader.Metadata(),
					TokenCount:   estimateTokens(content),
					Index:        idx,
				}
					idx++

					select {
					case chunkCh <- chunk:
					case <-ctx.Done():
						errCh <- ctx.Err()
						return
					}

					window = window[step:]
				}
			}
		}

		if len(window) > 0 {
			content := strings.Join(window, " ")
			chunk := types.Chunk{
				ID:           fmt.Sprintf("%s-chunk-%04d", docPath, idx),
				DocumentPath: docPath,
				Content:      content,
				Metadata:     reader.Metadata(),
				TokenCount:   estimateTokens(content),
				Index:        idx,
			}
			select {
			case chunkCh <- chunk:
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
		}
	}()

	return chunkCh, errCh
}

func init() {
	RegisterChunker("fixed", func(cfg map[string]any) (Chunker, error) {
		size := getInt(cfg, "size", 512)
		overlap := getInt(cfg, "overlap", 64)
		if size <= 0 {
			return nil, fmt.Errorf("fixed chunker: size must be > 0")
		}
		return NewFixedChunker(size, overlap), nil
	})
}

func estimateTokens(text string) int {
	return len(text) / 4
}
