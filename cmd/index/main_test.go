package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func resetFlags() {
	flag.CommandLine = flag.NewFlagSet("test", flag.ExitOnError)
}

func TestBuildIndexPipeline_StageNames(t *testing.T) {
	cfg := &config.Config{
		RepoURL:        "https://example.com/repo",
		RepoPath:       t.TempDir(),
		OutputPath:     t.TempDir(),
		MaxRetries:     3,
		RetryBackoff:   5,
		LogLevel:       "info",
		ChunkStrategy:  "fixed",
		ChunkSize:      512,
		ChunkOverlap:   64,
		EmbeddingModel: "text-embedding-3-small",
		BatchSize:      20,
		LLMBaseURL:     "https://api.openai.com/v1",
	}
	p := buildPipeline(cfg)

	require.Len(t, p.Stages, 4)

	expected := []string{"parse", "chunk", "embed", "store"}
	for i, name := range expected {
		assert.Equal(t, name, string(p.Stages[i].Name), "Stages[%d].Name", i)
	}
}

func TestBuildIndexPipeline_ConfigPtr(t *testing.T) {
	cfg := &config.Config{
		RepoURL:    "http://example.com",
		RepoPath:   t.TempDir(),
		OutputPath: t.TempDir(),
	}
	p := buildPipeline(cfg)
	assert.Equal(t, cfg, p.Config)
}

func TestBuildIndexPipeline_JournalDir(t *testing.T) {
	wd, _ := os.Getwd()
	journalDir := filepath.Join(wd, ".journal-index")

	os.RemoveAll(journalDir)

	cfg := &config.Config{
		RepoURL:      "http://example.com/repo",
		RepoPath:     t.TempDir(),
		OutputPath:   t.TempDir(),
		MaxRetries:   0,
		RetryBackoff: 1,
	}
	p := buildPipeline(cfg)

	require.NotNil(t, p.Journal, "Journal should not be nil")
}

func TestIndexPipeline_StageOrder(t *testing.T) {
	cfg := &config.Config{
		RepoURL:    "http://example.com/repo",
		RepoPath:   t.TempDir(),
		OutputPath: t.TempDir(),
	}
	p := buildPipeline(cfg)

	require.Len(t, p.Stages, 4)
	assert.Equal(t, "parse", string(p.Stages[0].Name))
	assert.Equal(t, "chunk", string(p.Stages[1].Name))
	assert.Equal(t, "embed", string(p.Stages[2].Name))
	assert.Equal(t, "store", string(p.Stages[3].Name))
}

func TestIndexPipeline_StageRequires(t *testing.T) {
	cfg := &config.Config{
		RepoURL:    "http://example.com/repo",
		RepoPath:   t.TempDir(),
		OutputPath: t.TempDir(),
	}
	p := buildPipeline(cfg)

	require.Len(t, p.Stages, 4)
	assert.Empty(t, p.Stages[0].Requires)
	assert.Equal(t, []types.StageID{"parse"}, p.Stages[1].Requires)
	assert.Equal(t, []types.StageID{"chunk"}, p.Stages[2].Requires)
	assert.Equal(t, []types.StageID{"embed"}, p.Stages[3].Requires)
}

func TestIndexPipeline_FromFlag(t *testing.T) {
	got := extractFromFlag([]string{"--from", "chunk"})
	assert.Equal(t, "chunk", got)
}

func TestIndexPipeline_InvalidFromFlag(t *testing.T) {
	resetFlags()

	tmpDir := t.TempDir()
	err := run([]string{
		"--from", "nonexistent",
		"--output", tmpDir,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"nonexistent"`)
}

func TestRun_ConfigError(t *testing.T) {
	resetFlags()

	tmpDir := t.TempDir()
	err := run([]string{
		"--output", tmpDir,
		"--chunk-strategy", "invalid",
	})
	assert.Error(t, err)
}
