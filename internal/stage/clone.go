package stageimport

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/pipeline"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func CloneStage(cfg *config.Config) pipeline.Stage {
	return pipeline.Stage{
		Name: "clone",
		Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
			repoPath := cfg.RepoPath
			repoURL := cfg.RepoURL
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
	cmd := exec.CommandContext(ctx, "git", "clone", url, path)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

func gitUpdate(ctx context.Context, path string) error {
	cmds := []struct {
		args []string
		desc string
	}{
		{[]string{"fetch", "--all"}, "git fetch"},
		{[]string{"checkout", "main"}, "git checkout main"},
		{[]string{"pull", "--ff-only"}, "git pull --ff-only"},
	}
	for _, c := range cmds {
		cmd := exec.CommandContext(ctx, "git", c.args...)
		cmd.Dir = path
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: %w", c.desc, err)
		}
	}
	return nil
}
