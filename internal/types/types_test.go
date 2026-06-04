package types

import (
	"errors"
	"testing"

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

func TestStageResultNilOutput(t *testing.T) {
	result := StageResult{Name: "verify"}
	assert.Nil(t, result.Output)
}
