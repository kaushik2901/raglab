package stageimport

import (
	"context"
	"fmt"
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
	cmd := exec.CommandContext(ctx, "git", "-c", "core.longpaths=true", "clone", "--depth", "1", "--single-branch", url, path)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
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
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = path
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", desc, err)
	}
	return nil
}
