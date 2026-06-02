package stageimport

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
)

func TestVerifyStage_Execute(t *testing.T) {
	srcDir := t.TempDir()
	contentDir := filepath.Join(srcDir, "content")
	os.MkdirAll(contentDir, 0755)
	os.WriteFile(filepath.Join(contentDir, "good.md"), []byte("# Valid\n\nSome content."), 0644)

	dstDir := t.TempDir()
	os.WriteFile(filepath.Join(dstDir, "good.md"), []byte("# Valid\n\nSome content."), 0644)

	cfg := &config.Config{
		OutputPath: dstDir,
	}
	stage := VerifyStage(cfg)

	result, err := stage.Run(context.Background(), map[string]any{
		"repo_path": srcDir,
	})
	if err != nil {
		t.Fatalf("VerifyStage.Run: %v", err)
	}
	if result.Err != nil {
		t.Fatalf("result.Err = %v", result.Err)
	}

	reportPath := filepath.Join(dstDir, "_verification_report.json")
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		t.Fatal("verification report not written")
	}
}

func TestVerifyStage_EmptyDir(t *testing.T) {
	srcDir := t.TempDir()
	os.MkdirAll(filepath.Join(srcDir, "content"), 0755)
	dstDir := t.TempDir()

	cfg := &config.Config{
		OutputPath: dstDir,
	}
	stage := VerifyStage(cfg)

	result, err := stage.Run(context.Background(), map[string]any{
		"repo_path": srcDir,
	})
	if err != nil {
		t.Fatalf("VerifyStage.Run: %v", err)
	}
	if result.Err != nil {
		t.Fatalf("result.Err = %v", result.Err)
	}

	reportPath := filepath.Join(dstDir, "_verification_report.json")
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		t.Fatal("verification report not written")
	}
}

func TestVerifyStage_Name(t *testing.T) {
	cfg := &config.Config{OutputPath: t.TempDir()}
	stage := VerifyStage(cfg)
	if stage.Name != "verify" {
		t.Errorf("got %q, want %q", stage.Name, "verify")
	}
}

func TestVerifyStage_Requires(t *testing.T) {
	cfg := &config.Config{OutputPath: t.TempDir()}
	stage := VerifyStage(cfg)
	expected := []string{"clone", "preprocess"}
	if len(stage.Requires) != 2 {
		t.Fatalf("Requires = %v, want %v", stage.Requires, expected)
	}
	for i, r := range stage.Requires {
		if string(r) != expected[i] {
			t.Errorf("Requires[%d] = %q, want %q", i, r, expected[i])
		}
	}
}
