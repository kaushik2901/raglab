package types

import "time"

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
