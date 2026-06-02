package pipeline

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type mockJournal struct {
	mu      sync.Mutex
	records map[types.StageID]*types.StageRecord
}

func newMockJournal() *mockJournal {
	return &mockJournal{records: make(map[types.StageID]*types.StageRecord)}
}

func (m *mockJournal) Record(stage types.StageID, record types.StageRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := record
	m.records[stage] = &cp
	return nil
}

func (m *mockJournal) Load(stage types.StageID) (*types.StageRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[stage]
	if !ok {
		return nil, nil
	}
	cp := *r
	return &cp, nil
}

func (m *mockJournal) HasSucceeded(stage types.StageID, inputHash string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[stage]
	if !ok {
		return false, nil
	}
	if !r.Succeeded {
		return false, nil
	}
	if inputHash == "" {
		return true, nil
	}
	return r.InputHash == inputHash, nil
}

func (m *mockJournal) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = make(map[types.StageID]*types.StageRecord)
	return nil
}

func testConfig() *config.Config {
	return &config.Config{
		MaxRetries:   2,
		RetryBackoff: time.Nanosecond,
	}
}

func identityStage(name types.StageID, requires ...types.StageID) Stage {
	return Stage{
		Name: name,
		Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
			return &types.StageResult{Name: name, Output: map[string]any{"ran": name}}, nil
		},
		Requires: requires,
	}
}

func TestRun_SingleStage(t *testing.T) {
	p := &Pipeline{
		Stages:  []Stage{identityStage("test")},
		Journal: newMockJournal(),
		Config:  testConfig(),
	}
	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRun_MultipleStages(t *testing.T) {
	order := make([]types.StageID, 0)
	p := &Pipeline{
		Stages: []Stage{
			{
				Name: "a",
				Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
					order = append(order, "a")
					return &types.StageResult{Name: "a", Output: map[string]any{"val": 1}}, nil
				},
			},
			{
				Name: "b",
				Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
					order = append(order, "b")
					if state["val"] != 1 {
						t.Errorf("state[val] = %v, want 1", state["val"])
					}
					return &types.StageResult{Name: "b", Output: map[string]any{"val2": 2}}, nil
				},
			},
		},
		Journal: newMockJournal(),
		Config:  testConfig(),
	}
	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Errorf("order = %v, want [a b]", order)
	}
}

func TestRun_DependencyMet(t *testing.T) {
	p := &Pipeline{
		Stages: []Stage{
			identityStage("a"),
			identityStage("b", "a"),
		},
		Journal: newMockJournal(),
		Config:  testConfig(),
	}
	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRun_DependencyNotMet(t *testing.T) {
	p := &Pipeline{
		Stages: []Stage{
			identityStage("b", "a"),
		},
		Journal: newMockJournal(),
		Config:  testConfig(),
	}
	err := p.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for unmet dependency")
	}
}

func TestRun_SkipCachedStage(t *testing.T) {
	runCount := 0
	p := &Pipeline{
		Stages: []Stage{
			{
				Name: "a",
				Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
					runCount++
					return &types.StageResult{Name: "a", Output: map[string]any{"val": 1}}, nil
				},
			},
		},
		Journal: newMockJournal(),
		Config:  testConfig(),
	}

	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if runCount != 1 {
		t.Errorf("runCount after first run = %d, want 1", runCount)
	}

	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if runCount != 1 {
		t.Errorf("runCount after second run = %d, want 1 (should be cached)", runCount)
	}
}

func TestRun_RetryThenSuccess(t *testing.T) {
	attempts := 0
	p := &Pipeline{
		Stages: []Stage{
			{
				Name: "flaky",
				Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
					attempts++
					if attempts < 2 {
						return nil, errors.New("transient error")
					}
					return &types.StageResult{Name: "flaky", Output: map[string]any{"ok": true}}, nil
				},
			},
		},
		Journal: newMockJournal(),
		Config:  &config.Config{MaxRetries: 2, RetryBackoff: time.Nanosecond},
	}
	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
}

func TestRun_RetryExhausted(t *testing.T) {
	attempts := 0
	p := &Pipeline{
		Stages: []Stage{
			{
				Name: "always-fails",
				Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
					attempts++
					return nil, errors.New("persistent error")
				},
			},
		},
		Journal: newMockJournal(),
		Config:  &config.Config{MaxRetries: 2, RetryBackoff: time.Nanosecond},
	}
	err := p.Run(context.Background())
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3 (1 initial + 2 retries)", attempts)
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Pipeline{
		Stages: []Stage{
			{
				Name: "slow",
				Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
					cancel()
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(10 * time.Second):
						return nil, nil
					}
				},
			},
		},
		Journal: newMockJournal(),
		Config:  &config.Config{MaxRetries: 0, RetryBackoff: time.Nanosecond},
	}
	err := p.Run(ctx)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestRunFrom_Resume(t *testing.T) {
	j := newMockJournal()
	j.Record("a", types.StageRecord{
		Name: "a", Succeeded: true, InputHash: computeInputHash(identityStage("a")),
		Output: map[string]any{"val": 1},
	})

	runB := false
	p := &Pipeline{
		Stages: []Stage{
			identityStage("a"),
			{
				Name: "b",
				Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
					runB = true
					if state["val"] != 1 {
						t.Errorf("state[val] = %v, want 1", state["val"])
					}
					return &types.StageResult{Name: "b"}, nil
				},
			},
		},
		Journal: j,
		Config:  testConfig(),
	}
	if err := p.RunFrom(context.Background(), "b"); err != nil {
		t.Fatalf("RunFrom: %v", err)
	}
	if !runB {
		t.Error("stage b was not executed")
	}
}

func TestRunFrom_UnknownStage(t *testing.T) {
	p := &Pipeline{
		Stages:  []Stage{identityStage("a")},
		Journal: newMockJournal(),
		Config:  testConfig(),
	}
	err := p.RunFrom(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown stage")
	}
}

func TestRun_EmptyStages(t *testing.T) {
	p := &Pipeline{
		Stages:  []Stage{},
		Journal: newMockJournal(),
		Config:  testConfig(),
	}
	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestProgressCallbacks(t *testing.T) {
	var mu sync.Mutex
	events := make([]string, 0)

	p := &Pipeline{
		Stages: []Stage{
			identityStage("a"),
			identityStage("b"),
		},
		Journal: newMockJournal(),
		Config:  testConfig(),
		OnProgress: func(name types.StageID, status string, progress float64) {
			mu.Lock()
			events = append(events, string(name)+":"+status)
			mu.Unlock()
		},
	}
	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) < 4 {
		t.Fatalf("too few progress events: %d", len(events))
	}
	if events[0] != ":initialized" {
		t.Errorf("first event = %q, want %q", events[0], ":initialized")
	}
	if events[len(events)-1] != ":completed" {
		t.Errorf("last event = %q, want %q", events[len(events)-1], ":completed")
	}
}

func TestRunFrom_FirstStage(t *testing.T) {
	runA := false
	p := &Pipeline{
		Stages: []Stage{
			{
				Name: "a",
				Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
					runA = true
					return &types.StageResult{Name: "a"}, nil
				},
			},
		},
		Journal: newMockJournal(),
		Config:  testConfig(),
	}
	if err := p.RunFrom(context.Background(), "a"); err != nil {
		t.Fatalf("RunFrom: %v", err)
	}
	if !runA {
		t.Error("stage a was not executed")
	}
}

func TestComputeInputHash_Deterministic(t *testing.T) {
	s1 := identityStage("test", "dep1", "dep2")
	s2 := identityStage("test", "dep1", "dep2")

	h1 := computeInputHash(s1)
	h2 := computeInputHash(s2)
	if h1 != h2 {
		t.Errorf("hashes differ: %q vs %q", h1, h2)
	}
}

func TestComputeInputHash_Different(t *testing.T) {
	s1 := identityStage("stage-a")
	s2 := identityStage("stage-b")

	h1 := computeInputHash(s1)
	h2 := computeInputHash(s2)
	if h1 == h2 {
		t.Errorf("expected different hashes, got same: %q", h1)
	}
}

func TestComputeInputHash_RequiresOrderIndependent(t *testing.T) {
	s1 := identityStage("test", "dep-b", "dep-a")
	s2 := identityStage("test", "dep-a", "dep-b")

	h1 := computeInputHash(s1)
	h2 := computeInputHash(s2)
	if h1 != h2 {
		t.Errorf("hashes should be order-independent: %q vs %q", h1, h2)
	}
}

func TestComputeInputHash_DifferentRequires(t *testing.T) {
	s1 := identityStage("test", "dep1")
	s2 := identityStage("test", "dep2")

	h1 := computeInputHash(s1)
	h2 := computeInputHash(s2)
	if h1 == h2 {
		t.Errorf("expected different hashes for different deps")
	}
}

func TestRun_SharedStateBetweenStages(t *testing.T) {
	p := &Pipeline{
		Stages: []Stage{
			{
				Name: "a",
				Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
					return &types.StageResult{Name: "a", Output: map[string]any{"x": 10}}, nil
				},
			},
			{
				Name: "b",
				Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
					if state["x"] != 10 {
						t.Errorf("state[x] = %v, want 10", state["x"])
					}
					return &types.StageResult{Name: "b", Output: map[string]any{"y": state["x"].(int) * 2}}, nil
				},
			},
		},
		Journal: newMockJournal(),
		Config:  testConfig(),
	}
	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	j, ok := p.Journal.(*mockJournal)
	if !ok {
		t.Fatal("expected mockJournal")
	}
	rec, _ := j.Load("b")
	if rec == nil {
		t.Fatal("no record for stage b")
	}
	if rec.Output["y"] != 20 {
		t.Errorf("state[y] = %v, want 20", rec.Output["y"])
	}
}

func TestRun_StageFailureSavesRecord(t *testing.T) {
	j := newMockJournal()
	p := &Pipeline{
		Stages: []Stage{
			{
				Name: "failing",
				Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
					return nil, errors.New("fail")
				},
			},
		},
		Journal: j,
		Config:  &config.Config{MaxRetries: 0, RetryBackoff: time.Nanosecond},
	}
	err := p.Run(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}

	rec, _ := j.Load("failing")
	if rec == nil {
		t.Fatal("expected record for failed stage")
	}
	if rec.Succeeded {
		t.Fatal("record should be marked as failed")
	}
}

func TestRun_EmptyPipeline(t *testing.T) {
	p := &Pipeline{
		Stages:  []Stage{},
		Journal: newMockJournal(),
		Config:  testConfig(),
	}
	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunFrom_ResumePreservesState(t *testing.T) {
	j := newMockJournal()
	j.Record("a", types.StageRecord{
		Name: "a", Succeeded: true,
		InputHash: computeInputHash(identityStage("a")),
		Output:    map[string]any{"repo_path": "/tmp/repo"},
	})

	stageBRan := false
	p := &Pipeline{
		Stages: []Stage{
			identityStage("a"),
			{
				Name: "b",
				Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
					stageBRan = true
					if state["repo_path"] != "/tmp/repo" {
						t.Errorf("state[repo_path] = %v, want /tmp/repo", state["repo_path"])
					}
					return &types.StageResult{Name: "b"}, nil
				},
			},
		},
		Journal: j,
		Config:  testConfig(),
	}
	if err := p.RunFrom(context.Background(), "b"); err != nil {
		t.Fatalf("RunFrom: %v", err)
	}
	if !stageBRan {
		t.Error("stage b did not run")
	}
}

func TestRun_RetryContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempt := 0
	p := &Pipeline{
		Stages: []Stage{
			{
				Name: "slow",
				Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
					attempt++
					if attempt == 1 {
						cancel()
					}
					return nil, errors.New("fail")
				},
			},
		},
		Journal: newMockJournal(),
		Config:  &config.Config{MaxRetries: 3, RetryBackoff: time.Millisecond},
	}
	err := p.Run(ctx)
	if err == nil {
		t.Fatal("expected error")
	}
}
