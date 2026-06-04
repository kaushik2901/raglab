package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCloneArgs_Kind(t *testing.T) {
	assert.Equal(t, "clone", CloneArgs{}.Kind())
}

func TestPreprocessArgs_Kind(t *testing.T) {
	assert.Equal(t, "preprocess", PreprocessArgs{}.Kind())
}

func TestVerifyArgs_Kind(t *testing.T) {
	assert.Equal(t, "verify", VerifyArgs{}.Kind())
}
