package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/qdrant/go-client/qdrant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func TestQdrant_Connect(t *testing.T) {
	t.Skip("requires Qdrant server")
	s := NewQdrantStore("")
	err := s.Connect(context.Background(), "http://localhost:6334")
	require.NoError(t, err)
	defer s.Close()
}

func TestQdrant_ConnectRefused(t *testing.T) {
	t.Skip("requires Qdrant server to reliably test connection refused")
	s := NewQdrantStore("")
	err := s.Connect(context.Background(), "http://localhost:1")
	assert.Error(t, err)
}

func TestQdrant_EnsureCollection_New(t *testing.T) {
	t.Skip("requires Qdrant server")
	s := NewQdrantStore("")
	err := s.Connect(context.Background(), "http://localhost:6334")
	require.NoError(t, err)
	defer s.Close()

	err = s.EnsureCollection(context.Background(), "test-collection-new", 4, "Cosine")
	assert.NoError(t, err)
}

func TestQdrant_EnsureCollection_Exists(t *testing.T) {
	t.Skip("requires Qdrant server")
	s := NewQdrantStore("")
	err := s.Connect(context.Background(), "http://localhost:6334")
	require.NoError(t, err)
	defer s.Close()

	err = s.EnsureCollection(context.Background(), "test-collection-exists", 4, "Cosine")
	require.NoError(t, err)

	err = s.EnsureCollection(context.Background(), "test-collection-exists", 4, "Cosine")
	assert.NoError(t, err)
}

func TestQdrant_Store_Basic(t *testing.T) {
	t.Skip("requires Qdrant server")
	s := NewQdrantStore("")
	err := s.Connect(context.Background(), "http://localhost:6334")
	require.NoError(t, err)
	defer s.Close()

	err = s.EnsureCollection(context.Background(), "test-store-basic", 3, "Cosine")
	require.NoError(t, err)

	chunks := []types.DocumentChunk{
		{
			Chunk: types.Chunk{
				ID:           "11111111-1111-1111-1111-111111111111",
				DocumentPath: "doc1.md",
				Content:      "test content",
				TokenCount:   3,
				Index:        0,
			},
			Embedding: types.Embedding{
				ChunkID:    "11111111-1111-1111-1111-111111111111",
				Vector:     []float64{0.1, 0.2, 0.3},
				Model:      "test-model",
				Dimensions: 3,
			},
		},
	}

	err = s.Store(context.Background(), "test-collection", chunks)
	assert.NoError(t, err)
}

func TestQdrant_Store_Empty(t *testing.T) {
	s := NewQdrantStore("")
	err := s.Store(context.Background(), "test-collection", nil)

	err = s.Store(context.Background(), "test-collection", []types.DocumentChunk{})
	assert.NoError(t, err)
}

func TestQdrant_Store_Batching(t *testing.T) {
	t.Skip("requires Qdrant server")
	s := NewQdrantStore("")
	err := s.Connect(context.Background(), "http://localhost:6334")
	require.NoError(t, err)
	defer s.Close()

	err = s.EnsureCollection(context.Background(), "test-store-batching", 1, "Cosine")
	require.NoError(t, err)

	chunks := make([]types.DocumentChunk, 250)
	for i := range chunks {
		chunks[i] = types.DocumentChunk{
			Chunk: types.Chunk{
				ID:           fmt.Sprintf("%08x-0000-0000-0000-%012x", i, i),
				DocumentPath: "doc.md",
				Content:      "content",
				TokenCount:   2,
				Index:        i,
			},
			Embedding: types.Embedding{
				ChunkID:    fmt.Sprintf("%08x-0000-0000-0000-%012x", i, i),
				Vector:     []float64{float64(i)},
				Model:      "m",
				Dimensions: 1,
			},
		}
	}

	err = s.Store(context.Background(), "test-store-batching", chunks)
	assert.NoError(t, err)
}

func TestQdrant_Store_PointConversion(t *testing.T) {
	t.Skip("point conversion verified via ToPoint tests")
}

func TestQdrant_Close(t *testing.T) {
	s := NewQdrantStore("")
	err := s.Close()
	assert.NoError(t, err)
}

func TestQdrant_Close_Idempotent(t *testing.T) {
	s := NewQdrantStore("")
	err := s.Close()
	assert.NoError(t, err)

	err = s.Close()
	assert.NoError(t, err)
}

func TestQdrant_ToPoint_ID(t *testing.T) {
	doc := types.DocumentChunk{
		Chunk: types.Chunk{
			ID: "test.md-chunk-0000",
		},
		Embedding: types.Embedding{
			Vector: []float64{0.1},
		},
	}

	point := toPoint(doc)
	require.NotNil(t, point)
	require.NotNil(t, point.Id)
	assert.Equal(t, chunkIDToUUID("test.md-chunk-0000"), point.Id.GetUuid())
}

func TestQdrant_ToPoint_Vector(t *testing.T) {
	doc := types.DocumentChunk{
		Chunk: types.Chunk{
			ID: "page.md-chunk-0001",
		},
		Embedding: types.Embedding{
			Vector: []float64{0.1, 0.2, 0.3},
		},
	}

	point := toPoint(doc)
	require.NotNil(t, point)
	require.NotNil(t, point.Vectors)

	v := point.Vectors.GetVector()
	require.NotNil(t, v)

	dense := v.GetDense()
	require.NotNil(t, dense)

	data := dense.GetData()
	require.Len(t, data, 3)
	assert.InDelta(t, float32(0.1), data[0], 0.0001)
	assert.InDelta(t, float32(0.2), data[1], 0.0001)
	assert.InDelta(t, float32(0.3), data[2], 0.0001)
}

func TestQdrant_ToPoint_Payload(t *testing.T) {
	doc := types.DocumentChunk{
		Chunk: types.Chunk{
			ID:           "cccccccc-cccc-cccc-cccc-cccccccccccc",
			DocumentPath: "handbook/docs/guide.md",
			Content:      "This is the chunk content.",
			TokenCount:   6,
			Index:        2,
		},
		Embedding: types.Embedding{
			ChunkID: "cccccccc-cccc-cccc-cccc-cccccccccccc",
			Vector:  []float64{0.1},
			Model:   "text-embedding-3-small",
		},
	}

	point := toPoint(doc)
	require.NotNil(t, point)
	require.NotNil(t, point.Payload)

	assert.Contains(t, point.Payload, "document_path")
	assert.Contains(t, point.Payload, "content")
	assert.Contains(t, point.Payload, "token_count")
	assert.Contains(t, point.Payload, "chunk_index")
	assert.Contains(t, point.Payload, "model")

	assert.Equal(t, "handbook/docs/guide.md", point.Payload["document_path"].GetStringValue())
	assert.Equal(t, "This is the chunk content.", point.Payload["content"].GetStringValue())
	assert.Equal(t, int64(6), point.Payload["token_count"].GetIntegerValue())
	assert.Equal(t, int64(2), point.Payload["chunk_index"].GetIntegerValue())
	assert.Equal(t, "text-embedding-3-small", point.Payload["model"].GetStringValue())
}

func TestChunkIDToUUID_Deterministic(t *testing.T) {
	u1 := chunkIDToUUID("doc.md-chunk-0000")
	u2 := chunkIDToUUID("doc.md-chunk-0000")
	assert.Equal(t, u1, u2)
}

func TestChunkIDToUUID_Different(t *testing.T) {
	u1 := chunkIDToUUID("doc1.md-chunk-0000")
	u2 := chunkIDToUUID("doc2.md-chunk-0000")
	assert.NotEqual(t, u1, u2)
}

func TestChunkIDToUUID_Format(t *testing.T) {
	u := chunkIDToUUID("")
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, u)
}

func TestParseDistance(t *testing.T) {
	assert.Equal(t, qdrant.Distance_Cosine, parseDistance("Cosine"))
	assert.Equal(t, qdrant.Distance_Euclid, parseDistance("Euclid"))
	assert.Equal(t, qdrant.Distance_Dot, parseDistance("Dot"))
	assert.Equal(t, qdrant.Distance_Manhattan, parseDistance("Manhattan"))
}

func TestParseDistance_Default(t *testing.T) {
	assert.Equal(t, qdrant.Distance_Cosine, parseDistance("Unknown"))
	assert.Equal(t, qdrant.Distance_Cosine, parseDistance(""))
}

func TestQdrant_ToPoint_PayloadTypes(t *testing.T) {
	doc := types.DocumentChunk{
		Chunk: types.Chunk{
			ID:           "dddddddd-dddd-dddd-dddd-dddddddddddd",
			DocumentPath: "path/to/doc.md",
			Content:      "some content",
			TokenCount:   42,
			Index:        1,
		},
		Embedding: types.Embedding{
			ChunkID: "dddddddd-dddd-dddd-dddd-dddddddddddd",
			Vector:  []float64{0.5},
			Model:   "test-model",
		},
	}

	point := toPoint(doc)
	require.NotNil(t, point)

	assert.IsType(t, &qdrant.Value{}, point.Payload["document_path"])
	assert.IsType(t, &qdrant.Value{}, point.Payload["content"])
	assert.IsType(t, &qdrant.Value{}, point.Payload["token_count"])
	assert.IsType(t, &qdrant.Value{}, point.Payload["chunk_index"])
	assert.IsType(t, &qdrant.Value{}, point.Payload["model"])

	assert.IsType(t, "", point.Payload["document_path"].GetStringValue())
	assert.IsType(t, "", point.Payload["content"].GetStringValue())
	assert.IsType(t, int64(0), point.Payload["token_count"].GetIntegerValue())
	assert.IsType(t, int64(0), point.Payload["chunk_index"].GetIntegerValue())
	assert.IsType(t, "", point.Payload["model"].GetStringValue())
}
