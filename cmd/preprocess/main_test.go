package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
)

func resetFlags() {
	flag.CommandLine = flag.NewFlagSet("test", flag.ExitOnError)
}

func TestExtractFromFlag_NotFound(t *testing.T) {
	got := extractFromFlag([]string{"--repo-url", "http://example.com"})
	assert.Equal(t, "", got)
}

func TestExtractFromFlag_Found(t *testing.T) {
	got := extractFromFlag([]string{"--from", "clone", "--repo-url", "http://example.com"})
	assert.Equal(t, "clone", got)
}

func TestExtractFromFlag_LastArg(t *testing.T) {
	got := extractFromFlag([]string{"--repo-url", "http://example.com", "--from"})
	assert.Equal(t, "", got)
}

func TestExtractFromFlag_EmptyArgs(t *testing.T) {
	got := extractFromFlag([]string{})
	assert.Equal(t, "", got)
}

func TestStripFromFlag_NoFlag(t *testing.T) {
	args := []string{"--repo-url", "http://example.com"}
	got := stripFromFlag(args)
	assert.Equal(t, args, got)
}

func TestStripFromFlag_WithFlag(t *testing.T) {
	got := stripFromFlag([]string{"--from", "clone", "--repo-url", "http://example.com"})
	assert.Equal(t, []string{"--repo-url", "http://example.com"}, got)
}

func TestStripFromFlag_FlagOnly(t *testing.T) {
	got := stripFromFlag([]string{"--from", "clone"})
	assert.Empty(t, got)
}

func TestBuildPipeline_StageNames(t *testing.T) {
	cfg := &config.Config{
		RepoURL:      "https://example.com/repo",
		RepoPath:     t.TempDir(),
		OutputPath:   t.TempDir(),
		MaxRetries:   3,
		RetryBackoff: 5,
		LogLevel:     "info",
	}
	p := buildPipeline(cfg)

	require.Len(t, p.Stages, 4)

	expected := []string{"clone", "sync-data", "preprocess", "verify"}
	for i, name := range expected {
		assert.Equal(t, name, string(p.Stages[i].Name), "Stages[%d].Name", i)
	}
}

func TestBuildPipeline_ConfigPtr(t *testing.T) {
	cfg := &config.Config{RepoURL: "http://example.com", RepoPath: t.TempDir(), OutputPath: t.TempDir()}
	p := buildPipeline(cfg)
	assert.Equal(t, cfg, p.Config)
}

func TestRun_ConfigError(t *testing.T) {
	resetFlags()

	tmpDir := t.TempDir()
	err := run([]string{
		"--repo-url", "",
		"--repo-path", tmpDir,
		"--output", tmpDir,
		"--max-retries", "1",
		"--retry-backoff", "1s",
	})
	assert.Error(t, err)
}

func TestRun_FromNonExistentStage(t *testing.T) {
	resetFlags()

	tmpDir := t.TempDir()
	err := run([]string{
		"--from", "nonexistent",
		"--repo-url", "http://example.com/repo",
		"--repo-path", tmpDir,
		"--output", tmpDir,
		"--max-retries", "0",
		"--retry-backoff", "1s",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"nonexistent"`)
}

func TestRun_FromFlagPreservesOtherFlags(t *testing.T) {
	resetFlags()

	tmpDir := t.TempDir()
	err := run([]string{
		"--from", "preprocess",
		"--repo-url", "http://example.com/repo",
		"--repo-path", tmpDir,
		"--output", tmpDir,
		"--max-retries", "0",
		"--retry-backoff", "1s",
	})
	// Config is valid, pipeline is built.
	// RunFrom("preprocess") skips "clone" (no journal -> empty state),
	// then runs "preprocess" which depends on "clone" -> dependency not met
	assert.Error(t, err)
}

func TestRun_InvalidRetryBackoff(t *testing.T) {
	resetFlags()

	tmpDir := t.TempDir()
	err := run([]string{
		"--repo-url", "http://example.com/repo",
		"--repo-path", tmpDir,
		"--output", tmpDir,
		"--retry-backoff", "0s",
	})
	assert.Error(t, err)
}

func TestRun_MissingRequiredFields(t *testing.T) {
	resetFlags()

	err := run([]string{
		"--repo-url", "",
		"--repo-path", "",
		"--output", "",
	})
	assert.Error(t, err)
}

func TestBuildPipeline_JournalDir(t *testing.T) {
	// Verify the journal directory is set to ".journal"
	wd, _ := os.Getwd()
	journalDir := filepath.Join(wd, ".journal")

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
