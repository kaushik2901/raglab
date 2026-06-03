package types

import "time"

type Workflow struct {
	ID          string
	Type        string
	Tag         string
	Status      string
	InputParams map[string]any
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type WorkflowStep struct {
	ID          string
	WorkflowID  string
	StepName    string
	Status      string
	Attempts    int
	Error       *string
	Output      map[string]any
	StartedAt   *time.Time
	CompletedAt *time.Time
	CreatedAt   time.Time
}
