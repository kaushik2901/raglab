package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

func PollUntilDone(ctx context.Context, store *Store, wfID string, interval time.Duration) error {
	tick := time.NewTicker(interval)
	defer tick.Stop()

	var lastStepLog string

	for {
		wf, err := store.GetWorkflow(ctx, wfID)
		if err != nil {
			return fmt.Errorf("poll workflow: %w", err)
		}

		steps, err := store.GetSteps(ctx, wfID)
		if err == nil {
			var done, total int
			for _, s := range steps {
				if s.Status == "succeeded" {
					done++
				}
				total++
			}
			progress := fmt.Sprintf("%d/%d", done, total)
			if progress != lastStepLog {
				slog.Info("workflow progress", "wf_id", wfID, "status", wf.Status, "steps", progress)
				lastStepLog = progress
			}
		}

		switch wf.Status {
		case "succeeded":
			slog.Info("workflow succeeded", "wf_id", wfID, "type", wf.Type, "tag", wf.Tag)
			return nil
		case "failed":
			for _, s := range steps {
				if s.Status == "failed" && s.Error != nil {
					slog.Error("workflow failed", "wf_id", wfID, "step", s.StepName, "err", *s.Error)
					return fmt.Errorf("step %s failed: %s", s.StepName, *s.Error)
				}
			}
			slog.Error("workflow failed", "wf_id", wfID)
			return fmt.Errorf("workflow %s failed", wfID)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}
