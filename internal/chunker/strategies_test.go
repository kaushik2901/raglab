package chunker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFutureChunkerStrategies_Registered(t *testing.T) {
	strategies := []string{"semantic", "recursive"}
	for _, name := range strategies {
		t.Run(name, func(t *testing.T) {
			_, err := New(name, 100, 20)
			assert.NoError(t, err, "%s chunker should be registered", name)
		})
	}
}
