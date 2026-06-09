package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// PollUntilTerminal polls a River job until it reaches a terminal state
// (completed, cancelled, or discarded). Returns the final JobRow on success.
func PollUntilTerminal(ctx context.Context, client *river.Client[pgx.Tx], jobID int64, interval time.Duration) (*rivertype.JobRow, error) {
	const maxInterval = 30 * time.Second
	currentInterval := interval
	if currentInterval <= 0 {
		currentInterval = time.Second
	}

	for {
		row, err := client.JobGet(ctx, jobID)
		if err != nil {
			return nil, fmt.Errorf("get job %d: %w", jobID, err)
		}

		switch row.State {
		case rivertype.JobStateCompleted:
			return row, nil
		case rivertype.JobStateCancelled, rivertype.JobStateDiscarded:
			return row, fmt.Errorf("job %d %s: %v", jobID, row.State, row.Errors)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(currentInterval):
		}

		currentInterval *= 2
		if currentInterval > maxInterval {
			currentInterval = maxInterval
		}
	}
}
