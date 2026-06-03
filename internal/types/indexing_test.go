package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChunkCreation(t *testing.T) {
	chunk := Chunk{
		ID:           "abc-123",
		DocumentPath: "docs/foo.md",
		Content:      "some content",
		Metadata:     map[string]string{"heading": "Intro"},
		TokenCount:   42,
		Index:        0,
	}
	assert.Equal(t, "abc-123", chunk.ID)
	assert.Equal(t, "docs/foo.md", chunk.DocumentPath)
	assert.Equal(t, "some content", chunk.Content)
	assert.Equal(t, "Intro", chunk.Metadata["heading"])
	assert.Equal(t, 42, chunk.TokenCount)
	assert.Equal(t, 0, chunk.Index)
}

func TestChunkZeroValue(t *testing.T) {
	var chunk Chunk
	assert.Equal(t, "", chunk.ID)
	assert.Equal(t, "", chunk.DocumentPath)
	assert.Equal(t, "", chunk.Content)
	assert.Nil(t, chunk.Metadata)
	assert.Equal(t, 0, chunk.TokenCount)
	assert.Equal(t, 0, chunk.Index)
}

func TestChunkMetadataNil(t *testing.T) {
	chunk := Chunk{ID: "test"}
	assert.Nil(t, chunk.Metadata, "default Metadata should be nil, not empty map")
}

func TestEmbeddingCreation(t *testing.T) {
	emb := Embedding{
		ChunkID:    "abc-123",
		Vector:     []float64{0.1, 0.2, 0.3},
		Model:      "text-embedding-3-small",
		Dimensions: 3,
	}
	assert.Equal(t, "abc-123", emb.ChunkID)
	assert.Equal(t, []float64{0.1, 0.2, 0.3}, emb.Vector)
	assert.Equal(t, "text-embedding-3-small", emb.Model)
	assert.Equal(t, 3, emb.Dimensions)
}

func TestEmbeddingZeroValue(t *testing.T) {
	var emb Embedding
	assert.Equal(t, "", emb.ChunkID)
	assert.Nil(t, emb.Vector)
	assert.Equal(t, "", emb.Model)
	assert.Equal(t, 0, emb.Dimensions)
}

func TestEmbeddingVectorType(t *testing.T) {
	vec := []float64{}
	vec = append(vec, 1.5, 2.5, 3.5)
	emb := Embedding{Vector: vec}
	assert.Equal(t, 3, len(emb.Vector))
	assert.Equal(t, 1.5, emb.Vector[0])
}

func TestDocumentChunkCreation(t *testing.T) {
	chunk := Chunk{ID: "chunk-1", Content: "hello", Index: 0}
	emb := Embedding{ChunkID: "chunk-1", Vector: []float64{0.1}, Model: "m1"}
	dc := DocumentChunk{Chunk: chunk, Embedding: emb}
	assert.Equal(t, "chunk-1", dc.Chunk.ID)
	assert.Equal(t, "chunk-1", dc.Embedding.ChunkID)
	assert.Equal(t, "m1", dc.Embedding.Model)
}

func TestDocumentChunkRoundTrip(t *testing.T) {
	chunk := Chunk{
		ID:           "doc1-chunk-0000",
		DocumentPath: "docs/page.md",
		Content:      "some text",
		TokenCount:   8,
		Index:        0,
	}
	emb := Embedding{
		ChunkID:    "doc1-chunk-0000",
		Vector:     []float64{0.01, 0.02},
		Model:      "text-embedding-3-small",
		Dimensions: 2,
	}
	dc := DocumentChunk{Chunk: chunk, Embedding: emb}
	assert.Equal(t, dc.Chunk.ID, dc.Embedding.ChunkID)
	assert.Equal(t, "docs/page.md", dc.Chunk.DocumentPath)
	assert.Equal(t, 2, dc.Embedding.Dimensions)
}
