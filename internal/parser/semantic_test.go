package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFutureParserStrategies_Registered(t *testing.T) {
	_, err := New("semantic")
	assert.NoError(t, err, "semantic parser should be registered")
}
