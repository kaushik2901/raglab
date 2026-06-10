package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndexArgs_Kind(t *testing.T) {
	assert.Equal(t, "index", IndexArgs{}.Kind())
}

func TestRunIndexing_EmptyInput(t *testing.T) {
	t.Run("returns no error for nonexistent input dir (walk warning)", func(t *testing.T) {
		err := RunIndexing(context.Background(), IndexArgs{
			Tag:               "test-collection",
			InputTag:          "nonexistent-tag-12345",
			EmbeddingProvider: "openai",
			EmbeddingModel:    "text-embedding-3-small",
			BatchSize:         10,
			IndexConcurrency:  5,
			ParserStrategy:    "markdown",
			ChunkStrategy:     "fixed",
			ChunkSize:         512,
			ChunkOverlap:      64,
			DocTimeout:        30 * time.Minute,
		})
		require.NoError(t, err)
	})
}

func TestMaxIndexFileSize_Constant(t *testing.T) {
	assert.Equal(t, 100*1024*1024, maxIndexFileSize)
}
