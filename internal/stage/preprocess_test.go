package stageimport

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
)

func TestPreprocessStage_Execute(t *testing.T) {
	srcDir := t.TempDir()
	contentDir := filepath.Join(srcDir, "content")
	os.MkdirAll(contentDir, 0755)
	os.WriteFile(filepath.Join(contentDir, "page.md"), []byte("# Hello"), 0644)

	dstDir := t.TempDir()

	cfg := &config.Config{
		OutputPath: dstDir,
	}
	stage := PreprocessStage(cfg)

	result, err := stage.Run(context.Background(), map[string]any{
		"repo_path": srcDir,
	})
	if err != nil {
		t.Fatalf("PreprocessStage.Run: %v", err)
	}
	if result.Err != nil {
		t.Fatalf("result.Err = %v", result.Err)
	}

	if _, err := os.Stat(filepath.Join(dstDir, "page.md")); os.IsNotExist(err) {
		t.Error("page.md not written to output")
	}
}

func TestPreprocessStage_Execute_EmptyDir(t *testing.T) {
	srcDir := t.TempDir()
	os.MkdirAll(filepath.Join(srcDir, "content"), 0755)
	dstDir := t.TempDir()

	cfg := &config.Config{
		OutputPath: dstDir,
	}
	stage := PreprocessStage(cfg)

	result, err := stage.Run(context.Background(), map[string]any{
		"repo_path": srcDir,
	})
	if err != nil {
		t.Fatalf("PreprocessStage.Run: %v", err)
	}
	if result.Err != nil {
		t.Fatalf("result.Err = %v", result.Err)
	}
}

func TestPreprocessStage_Name(t *testing.T) {
	cfg := &config.Config{OutputPath: t.TempDir()}
	stage := PreprocessStage(cfg)
	if stage.Name != "preprocess" {
		t.Errorf("got %q, want %q", stage.Name, "preprocess")
	}
}

func TestPreprocessStage_Requires(t *testing.T) {
	cfg := &config.Config{OutputPath: t.TempDir()}
	stage := PreprocessStage(cfg)
	if len(stage.Requires) != 1 || stage.Requires[0] != "clone" {
		t.Errorf("Requires = %v, want [clone]", stage.Requires)
	}
}
