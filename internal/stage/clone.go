package stage

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func CloneStage(repoURL, repoPath string) types.Stage {
	return types.Stage{
		Name: "clone",
		Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
			if _, err := os.Stat(repoPath); os.IsNotExist(err) {
				if err := gitClone(ctx, repoURL, repoPath); err != nil {
					return nil, fmt.Errorf("git clone: %w", err)
				}
			} else {
				if err := gitUpdate(ctx, repoPath); err != nil {
					return nil, fmt.Errorf("git update: %w", err)
				}
			}
			return &types.StageResult{
				Name:   "clone",
				Output: map[string]any{"repo_path": repoPath},
			}, nil
		},
	}
}

func gitClone(ctx context.Context, url, path string) error {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", "-c", "core.longpaths=true", "clone", "--depth", "1", "--single-branch", url, path)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone: %w\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	slog.Debug("git clone", "stdout", stdout.String(), "stderr", stderr.String())
	return nil
}

func gitUpdate(ctx context.Context, path string) error {
	if err := runGit(ctx, path, "git fetch --all", "fetch", "--all"); err != nil {
		return err
	}

	if err := runGit(ctx, path, "git checkout main", "checkout", "main"); err != nil {
		if err2 := runGit(ctx, path, "git checkout -b main origin/main", "checkout", "-b", "main", "origin/main"); err2 != nil {
			return fmt.Errorf("checkout main: %w (fetch fallback: %v)", err, err2)
		}
	}

	return runGit(ctx, path, "git pull --ff-only", "pull", "--ff-only")
}

func runGit(ctx context.Context, path, desc string, args ...string) error {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = path
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w\nstdout: %s\nstderr: %s", desc, err, stdout.String(), stderr.String())
	}
	slog.Debug(desc, "stdout", stdout.String(), "stderr", stderr.String())
	return nil
}
