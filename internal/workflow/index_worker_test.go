package workflow

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"

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
		})
		require.NoError(t, err)
	})
}

func TestIndexArgs_DocTimeoutDefault(t *testing.T) {
	t.Run("zero timeout defaults to 30 minutes", func(t *testing.T) {
		args := IndexArgs{DocTimeout: 0}
		timeout := args.DocTimeout
		if timeout <= 0 {
			timeout = 30 * time.Minute
		}
		assert.Equal(t, 30*time.Minute, timeout)
	})

	t.Run("positive timeout is preserved", func(t *testing.T) {
		args := IndexArgs{DocTimeout: 10 * time.Minute}
		timeout := args.DocTimeout
		if timeout <= 0 {
			timeout = 30 * time.Minute
		}
		assert.Equal(t, 10*time.Minute, timeout)
	})
}

func TestMaxIndexFileSize_EnvVar(t *testing.T) {
	orig := os.Getenv("MAX_INDEX_FILE_SIZE")
	os.Setenv("MAX_INDEX_FILE_SIZE", "50")
	defer os.Setenv("MAX_INDEX_FILE_SIZE", orig)

	maxSize := config.IntEnvOrDefault("MAX_INDEX_FILE_SIZE", 100*1024*1024)
	assert.Equal(t, 50, maxSize, "should read from env var")
}

func TestMaxIndexFileSize_Default(t *testing.T) {
	orig := os.Getenv("MAX_INDEX_FILE_SIZE")
	os.Unsetenv("MAX_INDEX_FILE_SIZE")
	defer os.Setenv("MAX_INDEX_FILE_SIZE", orig)

	maxSize := config.IntEnvOrDefault("MAX_INDEX_FILE_SIZE", 100*1024*1024)
	assert.Equal(t, 100*1024*1024, maxSize, "should use default when env unset")
}
