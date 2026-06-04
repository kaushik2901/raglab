package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIndexArgs_Kind(t *testing.T) {
	assert.Equal(t, "index", IndexArgs{}.Kind())
}
