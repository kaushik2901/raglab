package types

import (
	"context"
)

type StageID string

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
