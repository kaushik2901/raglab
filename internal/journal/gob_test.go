package journal

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func newTestJournal(t *testing.T) (*GobFileJournal, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "journal-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	return NewGobFileJournal(dir), dir
}

func cleanupJournal(t *testing.T, dir string) {
	t.Helper()
	os.RemoveAll(dir)
}

func TestNewGobFileJournal(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	if j.dir != dir {
		t.Errorf("dir = %q, want %q", j.dir, dir)
	}
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

	if err := j.Record("clone", record); err != nil {
		t.Fatalf("Record: %v", err)
	}

	got, err := j.Load("clone")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil {
		t.Fatal("Load returned nil, want non-nil")
	}
	if got.Name != record.Name {
		t.Errorf("Name = %q, want %q", got.Name, record.Name)
	}
	if got.Succeeded != record.Succeeded {
		t.Errorf("Succeeded = %t, want %t", got.Succeeded, record.Succeeded)
	}
	if got.InputHash != record.InputHash {
		t.Errorf("InputHash = %q, want %q", got.InputHash, record.InputHash)
	}
	if !got.StartedAt.Equal(record.StartedAt) {
		t.Errorf("StartedAt mismatch: %v vs %v", got.StartedAt, record.StartedAt)
	}
	if !got.FinishedAt.Equal(record.FinishedAt) {
		t.Errorf("FinishedAt mismatch: %v vs %v", got.FinishedAt, record.FinishedAt)
	}
	if v, ok := got.Output["repo_path"]; !ok || v != "/tmp/repo" {
		t.Errorf("Output[repo_path] = %v, want %v", v, "/tmp/repo")
	}
}

func TestLoad_MissingStage(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	got, err := j.Load("nonexistent")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != nil {
		t.Fatal("Load should return nil for missing stage")
	}
}

func TestHasSucceeded_True(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	record := types.StageRecord{Name: "clone", Succeeded: true, InputHash: "hash1"}
	if err := j.Record("clone", record); err != nil {
		t.Fatalf("Record: %v", err)
	}

	ok, err := j.HasSucceeded("clone", "hash1")
	if err != nil {
		t.Fatalf("HasSucceeded: %v", err)
	}
	if !ok {
		t.Fatal("HasSucceeded = false, want true")
	}
}

func TestHasSucceeded_NoRecord(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	ok, err := j.HasSucceeded("clone", "hash1")
	if err != nil {
		t.Fatalf("HasSucceeded: %v", err)
	}
	if ok {
		t.Fatal("HasSucceeded = true, want false")
	}
}

func TestHasSucceeded_WrongHash(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	record := types.StageRecord{Name: "clone", Succeeded: true, InputHash: "hash1"}
	if err := j.Record("clone", record); err != nil {
		t.Fatalf("Record: %v", err)
	}

	ok, err := j.HasSucceeded("clone", "hash2")
	if err != nil {
		t.Fatalf("HasSucceeded: %v", err)
	}
	if ok {
		t.Fatal("HasSucceeded = true, want false (hash mismatch)")
	}
}

func TestHasSucceeded_EmptyHashIgnoresHash(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	record := types.StageRecord{Name: "clone", Succeeded: true, InputHash: "some-specific-hash"}
	if err := j.Record("clone", record); err != nil {
		t.Fatalf("Record: %v", err)
	}

	ok, err := j.HasSucceeded("clone", "")
	if err != nil {
		t.Fatalf("HasSucceeded: %v", err)
	}
	if !ok {
		t.Fatal("HasSucceeded with empty inputHash = false, want true (should ignore hash)")
	}
}

func TestHasSucceeded_NotSucceeded(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	record := types.StageRecord{Name: "clone", Succeeded: false, InputHash: "hash1"}
	if err := j.Record("clone", record); err != nil {
		t.Fatalf("Record: %v", err)
	}

	ok, err := j.HasSucceeded("clone", "hash1")
	if err != nil {
		t.Fatalf("HasSucceeded: %v", err)
	}
	if ok {
		t.Fatal("HasSucceeded = true, want false (not succeeded)")
	}
}

func TestClear_RemovesGobFiles(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	if err := j.Record("clone", types.StageRecord{Name: "clone", Succeeded: true}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := j.Record("preprocess", types.StageRecord{Name: "preprocess", Succeeded: true}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	if err := j.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".gob" {
			t.Errorf("gob file remains after Clear: %s", e.Name())
		}
	}
}

func TestClear_LeavesNonGobFiles(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	if err := j.Record("clone", types.StageRecord{Name: "clone", Succeeded: true}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	nonGobPath := filepath.Join(dir, "readme.txt")
	if err := os.WriteFile(nonGobPath, []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := j.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	if _, err := os.Stat(nonGobPath); os.IsNotExist(err) {
		t.Fatal("Clear removed non-gob file readme.txt")
	}
}

func TestClear_NoDir(t *testing.T) {
	j := NewGobFileJournal(os.TempDir() + "/nonexistent-journal-" + time.Now().Format("150405.000"))

	if err := j.Clear(); err != nil {
		t.Fatalf("Clear on non-existent dir: %v", err)
	}
}

func TestRecord_OverwritesExisting(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	r1 := types.StageRecord{Name: "clone", Succeeded: true, InputHash: "hash1"}
	if err := j.Record("clone", r1); err != nil {
		t.Fatalf("Record: %v", err)
	}

	r2 := types.StageRecord{Name: "clone", Succeeded: false, InputHash: "hash2"}
	if err := j.Record("clone", r2); err != nil {
		t.Fatalf("Record: %v", err)
	}

	got, err := j.Load("clone")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Succeeded != false {
		t.Errorf("Succeeded = %t, want false (overwritten)", got.Succeeded)
	}
	if got.InputHash != "hash2" {
		t.Errorf("InputHash = %q, want %q", got.InputHash, "hash2")
	}
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

	if err := j.Record("test", record); err != nil {
		t.Fatalf("Record: %v", err)
	}

	got, err := j.Load("test")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil {
		t.Fatal("Load returned nil")
	}

	cases := []struct {
		key  string
		want any
	}{
		{"string_val", "hello"},
		{"int_val", 42},
		{"bool_val", true},
		{"float_val", 3.14},
	}
	for _, c := range cases {
		gotVal, ok := got.Output[c.key]
		if !ok {
			t.Errorf("Output[%q] missing", c.key)
			continue
		}
		if gotVal != c.want {
			t.Errorf("Output[%q] = %v (type: %T), want %v (type: %T)", c.key, gotVal, gotVal, c.want, c.want)
		}
	}
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
			if err := j.Record(stage, record); err != nil {
				t.Errorf("concurrent Record: %v", err)
			}
			if _, err := j.Load(stage); err != nil {
				t.Errorf("concurrent Load: %v", err)
			}
			if _, err := j.HasSucceeded(stage, "hash"); err != nil {
				t.Errorf("concurrent HasSucceeded: %v", err)
			}
		}(i)
	}
	wg.Wait()

	if err := j.Clear(); err != nil {
		t.Fatalf("Clear after concurrent access: %v", err)
	}
}

func TestGobEncoding_NilOutput(t *testing.T) {
	j, dir := newTestJournal(t)
	defer cleanupJournal(t, dir)

	record := types.StageRecord{Name: "test", Succeeded: true}
	if err := j.Record("test", record); err != nil {
		t.Fatalf("Record: %v", err)
	}

	got, err := j.Load("test")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil {
		t.Fatal("Load returned nil")
	}
	if got.Output != nil {
		t.Errorf("Output = %v, want nil", got.Output)
	}
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
		if err := j.Record(s.Name, s); err != nil {
			t.Fatalf("Record(%q): %v", s.Name, err)
		}
	}

	for _, want := range stages {
		got, err := j.Load(want.Name)
		if err != nil {
			t.Fatalf("Load(%q): %v", want.Name, err)
		}
		if got == nil {
			t.Fatalf("Load(%q) returned nil", want.Name)
		}
		if got.Succeeded != want.Succeeded {
			t.Errorf("Load(%q).Succeeded = %t, want %t", want.Name, got.Succeeded, want.Succeeded)
		}
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
			if err := j.Record(n, record); err != nil {
				t.Errorf("Record(%q): %v", n, err)
			}
			got, err := j.Load(n)
			if err != nil {
				t.Errorf("Load(%q): %v", n, err)
			}
			if got == nil {
				t.Errorf("Load(%q) returned nil", n)
			}
		}(name)
	}
	wg.Wait()
}
