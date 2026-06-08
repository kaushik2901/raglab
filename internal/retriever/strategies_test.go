package retriever

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFutureRetrieverStrategies_Registered(t *testing.T) {
	names := []string{"hybrid", "agentic"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			_, ok := strategies[name]
			assert.True(t, ok, "%s retriever should be registered", name)
		})
	}
}
