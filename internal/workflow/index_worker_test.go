package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseArgs_Kind(t *testing.T) {
	assert.Equal(t, "parse", ParseArgs{}.Kind())
}

func TestChunkArgs_Kind(t *testing.T) {
	assert.Equal(t, "chunk", ChunkArgs{}.Kind())
}

func TestEmbedArgs_Kind(t *testing.T) {
	assert.Equal(t, "embed", EmbedArgs{}.Kind())
}

func TestStoreArgs_Kind(t *testing.T) {
	assert.Equal(t, "store", StoreArgs{}.Kind())
}
