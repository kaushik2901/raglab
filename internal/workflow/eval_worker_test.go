package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEvalArgs_Kind(t *testing.T) {
	assert.Equal(t, "eval", EvalArgs{}.Kind())
}
