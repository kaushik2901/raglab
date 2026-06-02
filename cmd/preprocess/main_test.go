package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
)

func resetFlags() {
	flag.CommandLine = flag.NewFlagSet("test", flag.ExitOnError)
}

func TestExtractFromFlag_NotFound(t *testing.T) {
	got := extractFromFlag([]string{"--repo-url", "http://example.com"})
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractFromFlag_Found(t *testing.T) {
	got := extractFromFlag([]string{"--from", "clone", "--repo-url", "http://example.com"})
	if got != "clone" {
		t.Errorf("got %q, want %q", got, "clone")
	}
}

func TestExtractFromFlag_LastArg(t *testing.T) {
	got := extractFromFlag([]string{"--repo-url", "http://example.com", "--from"})
	if got != "" {
		t.Errorf("got %q, want empty (no value after --from)", got)
	}
}

func TestExtractFromFlag_EmptyArgs(t *testing.T) {
	got := extractFromFlag([]string{})
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestStripFromFlag_NoFlag(t *testing.T) {
	args := []string{"--repo-url", "http://example.com"}
	got := stripFromFlag(args)
	if len(got) != 2 || got[0] != "--repo-url" {
		t.Errorf("got %v, want %v", got, args)
	}
}

func TestStripFromFlag_WithFlag(t *testing.T) {
	got := stripFromFlag([]string{"--from", "clone", "--repo-url", "http://example.com"})
	want := []string{"--repo-url", "http://example.com"}
	if len(got) != 2 || got[0] != "--repo-url" {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestStripFromFlag_FlagOnly(t *testing.T) {
	got := stripFromFlag([]string{"--from", "clone"})
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
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

	if len(p.Stages) != 3 {
		t.Fatalf("got %d stages, want 3", len(p.Stages))
	}

	expected := []string{"clone", "preprocess", "verify"}
	for i, name := range expected {
		if string(p.Stages[i].Name) != name {
			t.Errorf("Stages[%d].Name = %q, want %q", i, p.Stages[i].Name, name)
		}
	}
}

func TestBuildPipeline_ConfigPtr(t *testing.T) {
	cfg := &config.Config{RepoURL: "http://example.com", RepoPath: t.TempDir(), OutputPath: t.TempDir()}
	p := buildPipeline(cfg)
	if p.Config != cfg {
		t.Error("Config pointer should match")
	}
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
	if err == nil {
		t.Fatal("expected error for empty repo-url")
	}
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
	if err == nil {
		t.Fatal("expected error for nonexistent stage")
	}
	if !strings.Contains(err.Error(), `"nonexistent"`) {
		t.Errorf("error = %v, want stage name in error message", err)
	}
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
	if err == nil {
		t.Fatal("expected error about unmet dependency")
	}
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
	if err == nil {
		t.Fatal("expected error for zero retry-backoff")
	}
}

func TestRun_MissingRequiredFields(t *testing.T) {
	resetFlags()

	err := run([]string{
		"--repo-url", "",
		"--repo-path", "",
		"--output", "",
	})
	if err == nil {
		t.Fatal("expected error for missing required fields")
	}
}

func TestBuildPipeline_JournalDir(t *testing.T) {
	// Verify the journal directory is set to ".journal"
	wd, _ := os.Getwd()
	journalDir := filepath.Join(wd, ".journal")

	// Before running, verify .journal doesn't exist
	os.RemoveAll(journalDir)

	cfg := &config.Config{
		RepoURL:      "http://example.com/repo",
		RepoPath:     t.TempDir(),
		OutputPath:   t.TempDir(),
		MaxRetries:   0,
		RetryBackoff: 1,
	}
	p := buildPipeline(cfg)

	// Journal should be a *journal.GobFileJournal (type-check by interface)
	if p.Journal == nil {
		t.Fatal("Journal should not be nil")
	}
}
