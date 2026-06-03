package workflow

import (
	"context"
	"fmt"
	"time"
)

func PollUntilDone(ctx context.Context, store *Store, wfID string, interval time.Duration) error {
	for {
		wf, err := store.GetWorkflow(ctx, wfID)
		if err != nil {
			return fmt.Errorf("poll workflow: %w", err)
		}

		switch wf.Status {
		case "succeeded":
			return nil
		case "failed":
			steps, err := store.GetSteps(ctx, wfID)
			if err != nil {
				return fmt.Errorf("fetch failed steps: %w", err)
			}
			for _, s := range steps {
				if s.Status == "failed" && s.Error != nil {
					return fmt.Errorf("step %s failed: %s", s.StepName, *s.Error)
				}
			}
			return fmt.Errorf("workflow %s failed", wfID)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}
