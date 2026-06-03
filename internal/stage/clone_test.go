package stageimport

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
)

func TestCloneStage_Execute_NewClone(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	srcDir := t.TempDir()
	cmd := exec.Command("git", "init", "--bare", filepath.Join(srcDir, ".git"))
	cmd.Dir = srcDir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git init --bare failed: %s", out)

	workDir := t.TempDir()
	cmd = exec.Command("git", "init")
	cmd.Dir = workDir
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, "git init failed: %s", out)

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
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, "git symbolic-ref failed: %s", out)

	cmd = exec.Command("git", "push", srcDir+"/.git", "master")
	cmd.Dir = workDir
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, "git push failed: %s", out)

	cfg := &config.Config{
		RepoURL:  srcDir + "/.git",
		RepoPath: filepath.Join(t.TempDir(), "repo"),
	}
	stage := CloneStage(cfg)

	result, err := stage.Run(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NoError(t, result.Err)
	assert.Equal(t, "clone", string(result.Name))

	_, err = os.Stat(filepath.Join(cfg.RepoPath, "test.md"))
	assert.False(t, os.IsNotExist(err), "test.md should be cloned")
}

func TestCloneStage_Name(t *testing.T) {
	cfg := &config.Config{
		RepoURL:  "https://example.com/repo.git",
		RepoPath: t.TempDir(),
	}
	stage := CloneStage(cfg)
	assert.Equal(t, "clone", string(stage.Name))
}
