package pipeline

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/journal"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type ProgressFunc func(name types.StageID, status string, progress float64)

type Stage struct {
	Name     types.StageID
	Run      func(ctx context.Context, state map[string]any) (*types.StageResult, error)
	Requires []types.StageID
}

type Pipeline struct {
	Stages     []Stage
	Journal    journal.Journal
	Config     *config.Config
	OnProgress ProgressFunc
}

func (p *Pipeline) Run(ctx context.Context) error {
	return p.runFrom(ctx, -1)
}

func (p *Pipeline) RunFrom(ctx context.Context, from types.StageID) error {
	startIdx := -1
	for i, s := range p.Stages {
		if s.Name == from {
			startIdx = i
			break
		}
	}
	if startIdx == -1 {
		return fmt.Errorf("stage %q not found", from)
	}
	return p.runFrom(ctx, startIdx)
}

func (p *Pipeline) runFrom(ctx context.Context, startIdx int) error {
	state := make(map[string]any)

	if p.OnProgress != nil {
		p.OnProgress("", "initialized", 0)
	}

	for i, stage := range p.Stages {
		if startIdx >= 0 && i < startIdx {
			record, err := p.Journal.Load(stage.Name)
			if err != nil {
				return fmt.Errorf("load journal for %s: %w", stage.Name, err)
			}
			if record != nil && record.Succeeded {
				for k, v := range record.Output {
					state[k] = v
				}
			}
			continue
		}

		for _, req := range stage.Requires {
			ok, err := p.Journal.HasSucceeded(req, "")
			if err != nil {
				return fmt.Errorf("check dependency %s for %s: %w", req, stage.Name, err)
			}
			if !ok {
				return fmt.Errorf("dependency %q not satisfied for stage %q", req, stage.Name)
			}
		}

		if p.OnProgress != nil {
			p.OnProgress(stage.Name, "running", float64(i)/float64(len(p.Stages)))
		}

		inputHash := computeInputHash(stage)

		cached, err := p.Journal.HasSucceeded(stage.Name, inputHash)
		if err != nil {
			return fmt.Errorf("check cache for %s: %w", stage.Name, err)
		}
		if cached {
			record, err := p.Journal.Load(stage.Name)
			if err != nil {
				return fmt.Errorf("load cached record for %s: %w", stage.Name, err)
			}
			if record != nil {
				for k, v := range record.Output {
					state[k] = v
				}
			}
			if p.OnProgress != nil {
				p.OnProgress(stage.Name, "cached", float64(i+1)/float64(len(p.Stages)))
			}
			continue
		}

		var result *types.StageResult
		startedAt := time.Now()

		err = p.retry(ctx, func() error {
			var runErr error
			result, runErr = stage.Run(ctx, state)
			return runErr
		})

		finishedAt := time.Now()

		record := types.StageRecord{
			Name:       stage.Name,
			Succeeded:  err == nil,
			Error:      errToString(err),
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			InputHash:  inputHash,
		}

		if result != nil {
			record.Output = result.Output
		}

		if saveErr := p.Journal.Record(stage.Name, record); saveErr != nil {
			return fmt.Errorf("save journal for %s: %w", stage.Name, saveErr)
		}

		if err != nil {
			return fmt.Errorf("stage %s failed after retries: %w", stage.Name, err)
		}

		if result != nil {
			for k, v := range result.Output {
				state[k] = v
			}
		}

		if p.OnProgress != nil {
			p.OnProgress(stage.Name, "completed", float64(i+1)/float64(len(p.Stages)))
		}
	}

	if p.OnProgress != nil {
		p.OnProgress("", "completed", 1)
	}
	return nil
}

func (p *Pipeline) retry(ctx context.Context, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt <= p.Config.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := float64(p.Config.RetryBackoff) * math.Pow(2, float64(attempt-1))
			half := int64(backoff) / 2
			if half <= 0 {
				half = 1
			}
			jitter := time.Duration(rand.Int63n(half))
			wait := time.Duration(backoff) + jitter

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}
	}
	return lastErr
}

func computeInputHash(stage Stage) string {
	h := sha256.New()
	h.Write([]byte(stage.Name))
	sort.Slice(stage.Requires, func(i, j int) bool {
		return stage.Requires[i] < stage.Requires[j]
	})
	for _, r := range stage.Requires {
		h.Write([]byte(r))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func errToString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
