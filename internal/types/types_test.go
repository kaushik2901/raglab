package types

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDocumentCreation(t *testing.T) {
	doc := Document{
		Path:    "content/docs/foo.md",
		Content: "# Hello\nWorld",
		Size:    18,
	}
	assert.Equal(t, "content/docs/foo.md", doc.Path)
	assert.Equal(t, "# Hello\nWorld", doc.Content)
	assert.Equal(t, int64(18), doc.Size)
}

func TestDocumentZeroValue(t *testing.T) {
	var doc Document
	assert.Equal(t, "", doc.Path)
	assert.Equal(t, "", doc.Content)
	assert.Equal(t, int64(0), doc.Size)
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
	assert.Equal(t, StageID("clone"), record.Name)
	assert.True(t, record.Succeeded)
	assert.Equal(t, "", record.Error)
	assert.True(t, record.StartedAt.Equal(now))
	assert.True(t, record.FinishedAt.Equal(now.Add(5*time.Second)))
	assert.Equal(t, "abc123", record.InputHash)
	assert.Equal(t, "/tmp/repo", record.Output["repo_path"])
}

func TestStageRecordFailedWithError(t *testing.T) {
	record := StageRecord{
		Name:      "preprocess",
		Succeeded: false,
		Error:     "something went wrong",
	}
	assert.False(t, record.Succeeded)
	assert.Equal(t, "something went wrong", record.Error)
}

func TestStageRecordNilOutput(t *testing.T) {
	record := StageRecord{Name: "verify"}
	assert.Nil(t, record.Output)
}

func TestStageResultCreation(t *testing.T) {
	result := StageResult{
		Name:   "clone",
		Output: map[string]any{"repo_path": "/tmp/repo"},
		Err:    nil,
	}
	assert.Equal(t, StageID("clone"), result.Name)
	assert.NoError(t, result.Err)
	assert.Equal(t, "/tmp/repo", result.Output["repo_path"])
}

func TestStageResultWithError(t *testing.T) {
	err := errors.New("network error")
	result := StageResult{
		Name: "clone",
		Err:  err,
	}
	assert.Error(t, result.Err)
	assert.Equal(t, "network error", result.Err.Error())
}

func TestStageIDType(t *testing.T) {
	var id StageID = "clone"
	assert.Equal(t, "clone", string(id))

	record := StageRecord{Name: id}
	assert.Equal(t, id, record.Name)
}

func TestStageResultNilOutput(t *testing.T) {
	result := StageResult{Name: "verify"}
	assert.Nil(t, result.Output)
}
