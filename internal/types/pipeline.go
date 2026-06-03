package types

import (
	"context"
	"time"
)

type StageID string

type StageRecord struct {
	Name       StageID
	Succeeded  bool
	Error      string
	StartedAt  time.Time
	FinishedAt time.Time
	InputHash  string
	Output     map[string]any
}

type StageResult struct {
	Name   StageID
	Output map[string]any
	Err    error
}

type Stage struct {
	Name     StageID
	Run      func(ctx context.Context, state map[string]any) (*StageResult, error)
	Requires []StageID
}
