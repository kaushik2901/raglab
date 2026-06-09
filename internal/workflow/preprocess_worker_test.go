package workflow

import (
	"testing"

	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"

	"github.com/riverqueue/river"
)

func TestPreprocessArgs_Kind(t *testing.T) {
	assert.Equal(t, "preprocess", PreprocessArgs{}.Kind())
}

func TestReadCheckpoint_Empty(t *testing.T) {
	job := &river.Job[PreprocessArgs]{
		JobRow: &rivertype.JobRow{},
	}
	cp := readCheckpoint(job)
	assert.NotNil(t, cp)
	assert.False(t, cp["clone_done"])
	assert.False(t, cp["preprocess_done"])
}
