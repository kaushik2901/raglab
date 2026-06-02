package types

import (
	"errors"
	"testing"
	"time"
)

func TestDocumentCreation(t *testing.T) {
	doc := Document{
		Path:    "content/docs/foo.md",
		Content: "# Hello\nWorld",
		Size:    18,
	}
	if doc.Path != "content/docs/foo.md" {
		t.Errorf("Path = %q, want %q", doc.Path, "content/docs/foo.md")
	}
	if doc.Content != "# Hello\nWorld" {
		t.Errorf("Content = %q, want %q", doc.Content, "# Hello\nWorld")
	}
	if doc.Size != 18 {
		t.Errorf("Size = %d, want %d", doc.Size, 18)
	}
}

func TestDocumentZeroValue(t *testing.T) {
	var doc Document
	if doc.Path != "" {
		t.Errorf("zero value Path = %q, want %q", doc.Path, "")
	}
	if doc.Content != "" {
		t.Errorf("zero value Content = %q, want %q", doc.Content, "")
	}
	if doc.Size != 0 {
		t.Errorf("zero value Size = %d, want %d", doc.Size, 0)
	}
}

func TestStageRecordCreation(t *testing.T) {
	now := time.Now()
	record := StageRecord{
		Name:       "clone",
		Succeeded:  true,
		Error:      "",
		StartedAt:  now,
		FinishedAt: now.Add(5 * time.Second),
		InputHash:  "abc123",
		Output:     map[string]any{"repo_path": "/tmp/repo"},
	}
	if record.Name != "clone" {
		t.Errorf("Name = %q, want %q", record.Name, "clone")
	}
	if !record.Succeeded {
		t.Error("Succeeded = false, want true")
	}
	if record.Error != "" {
		t.Errorf("Error = %q, want %q", record.Error, "")
	}
	if !record.StartedAt.Equal(now) {
		t.Errorf("StartedAt mismatch")
	}
	if !record.FinishedAt.Equal(now.Add(5 * time.Second)) {
		t.Errorf("FinishedAt mismatch")
	}
	if record.InputHash != "abc123" {
		t.Errorf("InputHash = %q, want %q", record.InputHash, "abc123")
	}
	if v, ok := record.Output["repo_path"]; !ok || v != "/tmp/repo" {
		t.Errorf("Output[repo_path] = %v, want %v", v, "/tmp/repo")
	}
}

func TestStageRecordFailedWithError(t *testing.T) {
	record := StageRecord{
		Name:      "preprocess",
		Succeeded: false,
		Error:     "something went wrong",
	}
	if record.Succeeded {
		t.Error("Succeeded = true, want false")
	}
	if record.Error != "something went wrong" {
		t.Errorf("Error = %q, want %q", record.Error, "something went wrong")
	}
}

func TestStageRecordNilOutput(t *testing.T) {
	record := StageRecord{Name: "verify"}
	if record.Output != nil {
		t.Error("Output should be nil for zero value")
	}
}

func TestStageResultCreation(t *testing.T) {
	result := StageResult{
		Name:   "clone",
		Output: map[string]any{"repo_path": "/tmp/repo"},
		Err:    nil,
	}
	if result.Name != "clone" {
		t.Errorf("Name = %q, want %q", result.Name, "clone")
	}
	if result.Err != nil {
		t.Errorf("Err = %v, want nil", result.Err)
	}
	if v, ok := result.Output["repo_path"]; !ok || v != "/tmp/repo" {
		t.Errorf("Output[repo_path] = %v, want %v", v, "/tmp/repo")
	}
}

func TestStageResultWithError(t *testing.T) {
	err := errors.New("network error")
	result := StageResult{
		Name: "clone",
		Err:  err,
	}
	if result.Err == nil {
		t.Fatal("Err = nil, want non-nil")
	}
	if result.Err.Error() != "network error" {
		t.Errorf("Err = %q, want %q", result.Err.Error(), "network error")
	}
}

func TestStageIDType(t *testing.T) {
	var id StageID = "clone"
	if string(id) != "clone" {
		t.Errorf("StageID = %q, want %q", string(id), "clone")
	}

	record := StageRecord{Name: id}
	if record.Name != id {
		t.Errorf("record.Name = %q, want %q", record.Name, id)
	}
}

func TestStageResultNilOutput(t *testing.T) {
	result := StageResult{Name: "verify"}
	if result.Output != nil {
		t.Error("Output should be nil for zero value")
	}
}
