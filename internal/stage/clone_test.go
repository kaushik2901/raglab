package stageimport

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
)

func TestCloneStage_Execute_NewClone(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	srcDir := t.TempDir()
	cmd := exec.Command("git", "init", "--bare", filepath.Join(srcDir, ".git"))
	cmd.Dir = srcDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}

	workDir := t.TempDir()
	cmd = exec.Command("git", "init")
	cmd.Dir = workDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = workDir
	cmd.CombinedOutput()
	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = workDir
	cmd.CombinedOutput()

	testFile := filepath.Join(workDir, "test.md")
	os.WriteFile(testFile, []byte("# Hello"), 0644)

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = workDir
	cmd.CombinedOutput()
	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = workDir
	cmd.CombinedOutput()

	cmd = exec.Command("git", "symbolic-ref", "HEAD", "refs/heads/master")
	cmd.Dir = srcDir + "/.git"
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git symbolic-ref: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "push", srcDir+"/.git", "master")
	cmd.Dir = workDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git push: %v\n%s", err, out)
	}

	cfg := &config.Config{
		RepoURL:  srcDir + "/.git",
		RepoPath: filepath.Join(t.TempDir(), "repo"),
	}
	stage := CloneStage(cfg)

	result, err := stage.Run(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("CloneStage.Run: %v", err)
	}
	if result.Name != "clone" {
		t.Errorf("Name = %q, want %q", result.Name, "clone")
	}
	if result.Err != nil {
		t.Fatalf("result.Err = %v", result.Err)
	}

	if _, err := os.Stat(filepath.Join(cfg.RepoPath, "test.md")); os.IsNotExist(err) {
		t.Error("test.md not cloned")
	}
}

func TestCloneStage_Name(t *testing.T) {
	cfg := &config.Config{
		RepoURL:  "https://example.com/repo.git",
		RepoPath: t.TempDir(),
	}
	stage := CloneStage(cfg)
	if stage.Name != "clone" {
		t.Errorf("got %q, want %q", stage.Name, "clone")
	}
}
