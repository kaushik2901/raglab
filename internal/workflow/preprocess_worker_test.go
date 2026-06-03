package workflow

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initLocalGitRepo(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0755))
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Stderr = os.Stderr
		require.NoError(t, cmd.Run())
	}
	contentDir := filepath.Join(dir, "content")
	require.NoError(t, os.MkdirAll(contentDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(contentDir, "test.md"), []byte("# Hello"), 0644))
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "branch", "-m", "main")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())
}

func TestCloneArgs_Kind(t *testing.T) {
	assert.Equal(t, "clone", CloneArgs{}.Kind())
}

func TestPreprocessArgs_Kind(t *testing.T) {
	assert.Equal(t, "preprocess", PreprocessArgs{}.Kind())
}

func TestVerifyArgs_Kind(t *testing.T) {
	assert.Equal(t, "verify", VerifyArgs{}.Kind())
}

func TestRunCloneStep_Success(t *testing.T) {
	srcRepo := t.TempDir()
	initLocalGitRepo(t, srcRepo)
	targetRepo := filepath.Join(t.TempDir(), "repo")

	args := CloneArgs{
		WorkflowID: "test-wf",
		Tag:        "pre-test",
		RepoURL:    srcRepo,
		RepoPath:   targetRepo,
	}

	result, err := RunCloneStep(context.Background(), args, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "clone", string(result.Name))
	assert.Equal(t, targetRepo, result.Output["repo_path"])
	assert.DirExists(t, filepath.Join(targetRepo, "content"))
}

func TestRunPreprocessStep_Success(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "content"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "content", "test.md"), []byte("# Hello World"), 0644))
	out := t.TempDir()

	args := PreprocessArgs{
		WorkflowID: "test-wf",
		Tag:        "pre-test",
		RepoPath:   repo,
		OutputPath: out,
	}

	state := map[string]any{"repo_path": repo}
	result, err := RunPreprocessStep(context.Background(), args, state)
	require.NoError(t, err)
	assert.Equal(t, "preprocess", string(result.Name))
	assert.Greater(t, result.Output["processed_count"], 0)
	assert.FileExists(t, filepath.Join(out, "test.md"))
}

func TestRunVerifyStep_Success(t *testing.T) {
	out := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(out, "test.md"), []byte("# Clean content"), 0644))

	args := VerifyArgs{
		WorkflowID: "test-wf",
		Tag:        "pre-test",
		RepoPath:   t.TempDir(),
		OutputPath: out,
	}

	state := map[string]any{"repo_path": t.TempDir()}
	result, err := RunVerifyStep(context.Background(), args, state)
	require.NoError(t, err)
	assert.Equal(t, "verify", string(result.Name))
	assert.FileExists(t, filepath.Join(out, "_verification_report.json"))
}
