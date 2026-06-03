package pipeline

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	assert.NoError(t, p.Run(context.Background()))
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
					assert.Equal(t, 1, state["val"])
					return &types.StageResult{Name: "b", Output: map[string]any{"val2": 2}}, nil
				},
			},
		},
		Journal: newMockJournal(),
		Config:  testConfig(),
	}
	require.NoError(t, p.Run(context.Background()))
	assert.Equal(t, []types.StageID{"a", "b"}, order)
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
	assert.NoError(t, p.Run(context.Background()))
}

func TestRun_DependencyNotMet(t *testing.T) {
	p := &Pipeline{
		Stages: []Stage{
			identityStage("b", "a"),
		},
		Journal: newMockJournal(),
		Config:  testConfig(),
	}
	assert.Error(t, p.Run(context.Background()))
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

	require.NoError(t, p.Run(context.Background()))
	assert.Equal(t, 1, runCount)

	require.NoError(t, p.Run(context.Background()))
	assert.Equal(t, 1, runCount, "should be cached on second run")
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
	require.NoError(t, p.Run(context.Background()))
	assert.Equal(t, 2, attempts)
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
	assert.Error(t, p.Run(context.Background()))
	assert.Equal(t, 3, attempts)
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
	assert.Error(t, p.Run(ctx))
}

func TestRunFrom_Resume(t *testing.T) {
	j := newMockJournal()
	require.NoError(t, j.Record("a", types.StageRecord{
		Name: "a", Succeeded: true, InputHash: computeInputHash(identityStage("a")),
		Output: map[string]any{"val": 1},
	}))

	runB := false
	p := &Pipeline{
		Stages: []Stage{
			identityStage("a"),
			{
				Name: "b",
				Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
					runB = true
					assert.Equal(t, 1, state["val"])
					return &types.StageResult{Name: "b"}, nil
				},
			},
		},
		Journal: j,
		Config:  testConfig(),
	}
	require.NoError(t, p.RunFrom(context.Background(), "b"))
	assert.True(t, runB, "stage b was not executed")
}

func TestRunFrom_UnknownStage(t *testing.T) {
	p := &Pipeline{
		Stages:  []Stage{identityStage("a")},
		Journal: newMockJournal(),
		Config:  testConfig(),
	}
	assert.Error(t, p.RunFrom(context.Background(), "nonexistent"))
}

func TestRun_EmptyStages(t *testing.T) {
	p := &Pipeline{
		Stages:  []Stage{},
		Journal: newMockJournal(),
		Config:  testConfig(),
	}
	assert.NoError(t, p.Run(context.Background()))
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
	require.NoError(t, p.Run(context.Background()))

	mu.Lock()
	defer mu.Unlock()
	assert.GreaterOrEqual(t, len(events), 4)
	assert.Equal(t, ":initialized", events[0])
	assert.Equal(t, ":completed", events[len(events)-1])
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
	require.NoError(t, p.RunFrom(context.Background(), "a"))
	assert.True(t, runA, "stage a was not executed")
}

func TestComputeInputHash_Deterministic(t *testing.T) {
	s1 := identityStage("test", "dep1", "dep2")
	s2 := identityStage("test", "dep1", "dep2")

	h1 := computeInputHash(s1)
	h2 := computeInputHash(s2)
	assert.Equal(t, h1, h2)
}

func TestComputeInputHash_Different(t *testing.T) {
	s1 := identityStage("stage-a")
	s2 := identityStage("stage-b")

	h1 := computeInputHash(s1)
	h2 := computeInputHash(s2)
	assert.NotEqual(t, h1, h2)
}

func TestComputeInputHash_RequiresOrderIndependent(t *testing.T) {
	s1 := identityStage("test", "dep-b", "dep-a")
	s2 := identityStage("test", "dep-a", "dep-b")

	h1 := computeInputHash(s1)
	h2 := computeInputHash(s2)
	assert.Equal(t, h1, h2)
}

func TestComputeInputHash_DifferentRequires(t *testing.T) {
	s1 := identityStage("test", "dep1")
	s2 := identityStage("test", "dep2")

	h1 := computeInputHash(s1)
	h2 := computeInputHash(s2)
	assert.NotEqual(t, h1, h2)
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
					assert.Equal(t, 10, state["x"])
					return &types.StageResult{Name: "b", Output: map[string]any{"y": state["x"].(int) * 2}}, nil
				},
			},
		},
		Journal: newMockJournal(),
		Config:  testConfig(),
	}
	require.NoError(t, p.Run(context.Background()))

	j, ok := p.Journal.(*mockJournal)
	require.True(t, ok)
	rec, err := j.Load("b")
	require.NoError(t, err)
	require.NotNil(t, rec)
	assert.Equal(t, 20, rec.Output["y"])
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
	assert.Error(t, p.Run(context.Background()))

	rec, err := j.Load("failing")
	require.NoError(t, err)
	require.NotNil(t, rec, "expected record for failed stage")
	assert.False(t, rec.Succeeded)
}

func TestRun_EmptyPipeline(t *testing.T) {
	p := &Pipeline{
		Stages:  []Stage{},
		Journal: newMockJournal(),
		Config:  testConfig(),
	}
	assert.NoError(t, p.Run(context.Background()))
}

func TestRunFrom_ResumePreservesState(t *testing.T) {
	j := newMockJournal()
	require.NoError(t, j.Record("a", types.StageRecord{
		Name: "a", Succeeded: true,
		InputHash: computeInputHash(identityStage("a")),
		Output:    map[string]any{"repo_path": "/tmp/repo"},
	}))

	stageBRan := false
	p := &Pipeline{
		Stages: []Stage{
			identityStage("a"),
			{
				Name: "b",
				Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
					stageBRan = true
					assert.Equal(t, "/tmp/repo", state["repo_path"])
					return &types.StageResult{Name: "b"}, nil
				},
			},
		},
		Journal: j,
		Config:  testConfig(),
	}
	require.NoError(t, p.RunFrom(context.Background(), "b"))
	assert.True(t, stageBRan, "stage b did not run")
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
	assert.Error(t, p.Run(ctx))
}
