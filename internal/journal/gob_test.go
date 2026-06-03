package journal

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func newTestJournal(t *testing.T) (*GobFileJournal, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "journal-test-*")
	require.NoError(t, err)
	return NewGobFileJournal(dir), dir
}

func cleanupJournal(t *testing.T, dir string) {
	t.Helper()
	os.RemoveAll(dir)
}

func TestNewGobFileJournal(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	assert.Equal(t, dir, j.dir)
}

func TestRecordAndLoad(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	now := time.Now().Truncate(time.Second)
	record := types.StageRecord{
		Name:       "clone",
		Succeeded:  true,
		Error:      "",
		StartedAt:  now,
		FinishedAt: now.Add(5 * time.Second),
		InputHash:  "abc123",
		Output:     map[string]any{"repo_path": "/tmp/repo"},
	}

	require.NoError(t, j.Record("clone", record))

	got, err := j.Load("clone")
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, record.Name, got.Name)
	assert.Equal(t, record.Succeeded, got.Succeeded)
	assert.Equal(t, record.InputHash, got.InputHash)
	assert.True(t, got.StartedAt.Equal(record.StartedAt))
	assert.True(t, got.FinishedAt.Equal(record.FinishedAt))
	v, ok := got.Output["repo_path"]
	assert.True(t, ok)
	assert.Equal(t, "/tmp/repo", v)
}

func TestLoad_MissingStage(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	got, err := j.Load("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestHasSucceeded_True(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	record := types.StageRecord{Name: "clone", Succeeded: true, InputHash: "hash1"}
	require.NoError(t, j.Record("clone", record))

	ok, err := j.HasSucceeded("clone", "hash1")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestHasSucceeded_NoRecord(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	ok, err := j.HasSucceeded("clone", "hash1")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestHasSucceeded_WrongHash(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	record := types.StageRecord{Name: "clone", Succeeded: true, InputHash: "hash1"}
	require.NoError(t, j.Record("clone", record))

	ok, err := j.HasSucceeded("clone", "hash2")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestHasSucceeded_EmptyHashIgnoresHash(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	record := types.StageRecord{Name: "clone", Succeeded: true, InputHash: "some-specific-hash"}
	require.NoError(t, j.Record("clone", record))

	ok, err := j.HasSucceeded("clone", "")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestHasSucceeded_NotSucceeded(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	record := types.StageRecord{Name: "clone", Succeeded: false, InputHash: "hash1"}
	require.NoError(t, j.Record("clone", record))

	ok, err := j.HasSucceeded("clone", "hash1")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestClear_RemovesGobFiles(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	require.NoError(t, j.Record("clone", types.StageRecord{Name: "clone", Succeeded: true}))
	require.NoError(t, j.Record("preprocess", types.StageRecord{Name: "preprocess", Succeeded: true}))

	require.NoError(t, j.Clear())

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotEqual(t, ".gob", filepath.Ext(e.Name()), "gob file remains after Clear: %s", e.Name())
	}
}

func TestClear_LeavesNonGobFiles(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	require.NoError(t, j.Record("clone", types.StageRecord{Name: "clone", Succeeded: true}))

	nonGobPath := filepath.Join(dir, "readme.txt")
	require.NoError(t, os.WriteFile(nonGobPath, []byte("hello"), 0644))

	require.NoError(t, j.Clear())

	_, err := os.Stat(nonGobPath)
	assert.False(t, os.IsNotExist(err), "Clear should not remove non-gob files")
}

func TestClear_NoDir(t *testing.T) {
	j := NewGobFileJournal(os.TempDir() + "/nonexistent-journal-" + time.Now().Format("150405.000"))
	assert.NoError(t, j.Clear())
}

func TestRecord_OverwritesExisting(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	r1 := types.StageRecord{Name: "clone", Succeeded: true, InputHash: "hash1"}
	require.NoError(t, j.Record("clone", r1))

	r2 := types.StageRecord{Name: "clone", Succeeded: false, InputHash: "hash2"}
	require.NoError(t, j.Record("clone", r2))

	got, err := j.Load("clone")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.False(t, got.Succeeded)
	assert.Equal(t, "hash2", got.InputHash)
}

func TestGobEncoding_ComplexOutput(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	record := types.StageRecord{
		Name:      "test",
		Succeeded: true,
		Output: map[string]any{
			"string_val": "hello",
			"int_val":    42,
			"bool_val":   true,
			"float_val":  3.14,
		},
	}

	require.NoError(t, j.Record("test", record))

	got, err := j.Load("test")
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, "hello", got.Output["string_val"])
	assert.Equal(t, 42, got.Output["int_val"])
	assert.Equal(t, true, got.Output["bool_val"])
	assert.Equal(t, 3.14, got.Output["float_val"])
}

func TestConcurrentAccess(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			stage := types.StageID("stage")
			record := types.StageRecord{
				Name:      stage,
				Succeeded: true,
				InputHash: "hash",
				Output:    map[string]any{"index": i},
			}
			assert.NoError(t, j.Record(stage, record))
			_, err := j.Load(stage)
			assert.NoError(t, err)
			_, err = j.HasSucceeded(stage, "hash")
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()

	assert.NoError(t, j.Clear())
}

func TestGobEncoding_NilOutput(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	record := types.StageRecord{Name: "test", Succeeded: true}
	require.NoError(t, j.Record("test", record))

	got, err := j.Load("test")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, got.Output)
}

func TestRecordAndLoadMultipleStages(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	stages := []types.StageRecord{
		{Name: "clone", Succeeded: true, InputHash: "h1"},
		{Name: "preprocess", Succeeded: true, InputHash: "h2"},
		{Name: "verify", Succeeded: false, Error: "failed", InputHash: "h3"},
	}

	for _, s := range stages {
		require.NoError(t, j.Record(s.Name, s))
	}

	for _, want := range stages {
		got, err := j.Load(want.Name)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, want.Succeeded, got.Succeeded)
	}
}

func TestConcurrentDifferentStages(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	var wg sync.WaitGroup
	stageNames := []types.StageID{"a", "b", "c", "d", "e"}
	for _, name := range stageNames {
		wg.Add(1)
		go func(n types.StageID) {
			defer wg.Done()
			record := types.StageRecord{Name: n, Succeeded: true, InputHash: "h"}
			assert.NoError(t, j.Record(n, record))
			got, err := j.Load(n)
			assert.NoError(t, err)
			assert.NotNil(t, got)
		}(name)
	}
	wg.Wait()
}
