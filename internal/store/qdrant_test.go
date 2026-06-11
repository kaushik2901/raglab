package store

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/qdrant/go-client/qdrant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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

func TestQdrantStore_LastDSN(t *testing.T) {
	s := NewQdrantStore("test-key")
	_ = s.Connect(context.Background(), "http://qdrant:6334")
	assert.Equal(t, "http://qdrant:6334", s.lastDSN)
}

func TestQdrantStore_StoreOnce_NotConnected(t *testing.T) {
	s := NewQdrantStore("")
	err := s.storeOnce(context.Background(), "test", []types.DocumentChunk{
		{Chunk: types.Chunk{ID: "c1"}, Embedding: types.Embedding{Vector: []float64{0.1}}},
	})
	assert.ErrorContains(t, err, "not connected")
}

func TestQdrantStore_SearchOnce_NotConnected(t *testing.T) {
	s := NewQdrantStore("")
	_, err := s.searchOnce(context.Background(), "test", nil, 5)
	assert.ErrorContains(t, err, "not connected")
}

func TestQdrantStore_EnsureCollectionOnce_NotConnected(t *testing.T) {
	s := NewQdrantStore("")
	err := s.ensureCollectionOnce(context.Background(), "test", 4, "Cosine")
	assert.ErrorContains(t, err, "not connected")
}

func TestQdrantStore_Reconnect_NoDSN(t *testing.T) {
	s := NewQdrantStore("")
	err := s.reconnect(context.Background())
	assert.ErrorContains(t, err, "no last DSN")
}

func TestIsConnError_Unavailable(t *testing.T) {
	st := status.New(codes.Unavailable, "service temporarily unavailable")
	assert.True(t, isConnError(st.Err()))
}

func TestIsConnError_DeadlineExceeded(t *testing.T) {
	st := status.New(codes.DeadlineExceeded, "deadline exceeded")
	assert.True(t, isConnError(st.Err()))
}

func TestIsConnError_Canceled(t *testing.T) {
	st := status.New(codes.Canceled, "context canceled")
	assert.True(t, isConnError(st.Err()))
}

func TestIsConnError_NotFound(t *testing.T) {
	st := status.New(codes.NotFound, "not found")
	assert.False(t, isConnError(st.Err()))
}

func TestIsConnError_PlainError(t *testing.T) {
	assert.False(t, isConnError(fmt.Errorf("some random error")))
}

func TestQdrantStore_Retry_NonConnError(t *testing.T) {
	s := NewQdrantStore("")
	// No client → storeOnce returns "not connected" which is NOT a conn error
	// so Store should return immediately without retry
	err := s.Store(context.Background(), "test", []types.DocumentChunk{
		{Chunk: types.Chunk{ID: "c1"}, Embedding: types.Embedding{Vector: []float64{0.1}}},
	})
	assert.ErrorContains(t, err, "not connected")
}

func TestQdrantStore_Retry_SearchNonConnError(t *testing.T) {
	s := NewQdrantStore("")
	_, err := s.Search(context.Background(), "test", []float32{0.1}, 5)
	assert.ErrorContains(t, err, "not connected")
}

func TestQdrantStore_Retry_EnsureCollectionNonConnError(t *testing.T) {
	s := NewQdrantStore("")
	err := s.EnsureCollection(context.Background(), "test", 4, "Cosine")
	assert.ErrorContains(t, err, "not connected")
}

func TestRetryWithBackoff_SuccessFirstAttempt(t *testing.T) {
	opCount := atomic.Int32{}
	err := retryWithBackoff(context.Background(), 3,
		func(ctx context.Context) error {
			opCount.Add(1)
			return nil
		},
		func() error { return nil },
	)
	require.NoError(t, err)
	assert.Equal(t, int32(1), opCount.Load(), "should succeed on first attempt")
}

func TestRetryWithBackoff_RetryOnConnError(t *testing.T) {
	opCount := atomic.Int32{}
	reconnectCount := atomic.Int32{}
	err := retryWithBackoff(context.Background(), 3,
		func(ctx context.Context) error {
			n := opCount.Add(1)
			if n < 3 {
				return status.Error(codes.Unavailable, "service unavailable")
			}
			return nil
		},
		func() error {
			reconnectCount.Add(1)
			return nil
		},
	)
	require.NoError(t, err)
	assert.Equal(t, int32(3), opCount.Load(), "should retry on conn errors")
	assert.Equal(t, int32(2), reconnectCount.Load(), "should reconnect before each retry")
}

func TestRetryWithBackoff_NonConnErrorFailsImmediately(t *testing.T) {
	opCount := atomic.Int32{}
	err := retryWithBackoff(context.Background(), 3,
		func(ctx context.Context) error {
			opCount.Add(1)
			return fmt.Errorf("bad request")
		},
		func() error { return nil },
	)
	require.Error(t, err)
	assert.Equal(t, int32(1), opCount.Load(), "should NOT retry non-connection error")
}

func TestRetryWithBackoff_ExhaustAttempts(t *testing.T) {
	opCount := atomic.Int32{}
	err := retryWithBackoff(context.Background(), 3,
		func(ctx context.Context) error {
			opCount.Add(1)
			return status.Error(codes.Unavailable, "still unavailable")
		},
		func() error { return nil },
	)
	require.Error(t, err)
	assert.Equal(t, int32(3), opCount.Load(), "should exhaust all 3 attempts")
}

func TestRetryWithBackoff_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opCount := atomic.Int32{}
	err := retryWithBackoff(ctx, 3,
		func(ctx context.Context) error {
			opCount.Add(1)
			return status.Error(codes.Unavailable, "unavailable")
		},
		func() error { return nil },
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int32(1), opCount.Load(), "should not retry after context cancel")
}

func TestRetryWithBackoff_ReconnectFailure(t *testing.T) {
	opCount := atomic.Int32{}
	reconnectCount := atomic.Int32{}
	err := retryWithBackoff(context.Background(), 3,
		func(ctx context.Context) error {
			opCount.Add(1)
			return status.Error(codes.Unavailable, "unavailable")
		},
		func() error {
			reconnectCount.Add(1)
			return fmt.Errorf("reconnect failed")
		},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reconnect failed")
	assert.Equal(t, int32(1), opCount.Load(), "operation should be attempted once")
	assert.Equal(t, int32(1), reconnectCount.Load(), "one reconnect attempt")
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
