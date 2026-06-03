package embedder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func TestEmbed_Basic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := embedResponse{
			Data: []embedData{
				{Index: 0, Embedding: []float64{0.1, 0.2, 0.3}},
				{Index: 1, Embedding: []float64{0.4, 0.5, 0.6}},
			},
			Model: "text-embedding-3-small",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := New(srv.URL, "", "text-embedding-3-small", 10)
	chunks := []types.Chunk{
		{ID: "c1", Content: "hello"},
		{ID: "c2", Content: "world"},
	}

	embeddings, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	assert.Len(t, embeddings, 2)
	assert.Len(t, embeddings[0].Vector, 3)
	assert.Equal(t, 0.1, embeddings[0].Vector[0])
}

func TestEmbed_Batching(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		var req embedRequest
		json.NewDecoder(r.Body).Decode(&req)
		data := make([]embedData, len(req.Input))
		for i := range req.Input {
			data[i] = embedData{Index: i, Embedding: []float64{float64(i)}}
		}
		json.NewEncoder(w).Encode(embedResponse{Data: data, Model: "m"})
	}))
	defer srv.Close()

	e := New(srv.URL, "", "m", 10)
	chunks := make([]types.Chunk, 25)
	for i := range chunks {
		chunks[i] = types.Chunk{ID: string(rune('a' + i)), Content: "x"}
	}

	embeddings, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	assert.Len(t, embeddings, 25)
	assert.Equal(t, int32(3), atomic.LoadInt32(&callCount))
}

func TestEmbed_EmptyInput(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	e := New(srv.URL, "", "m", 10)
	embeddings, err := e.Embed(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, embeddings)
	assert.False(t, called, "API should not be called for empty input")
}

func TestEmbed_ModelName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(embedResponse{
			Data:  []embedData{{Index: 0, Embedding: []float64{0.1}}},
			Model: "custom-model",
		})
	}))
	defer srv.Close()

	e := New(srv.URL, "", "custom-model", 10)
	chunks := []types.Chunk{{ID: "c1", Content: "x"}}

	embeddings, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	assert.Equal(t, "custom-model", embeddings[0].Model)
}

func TestEmbed_Dimensions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(embedResponse{
			Data:  []embedData{{Index: 0, Embedding: []float64{0.1, 0.2, 0.3, 0.4}}},
			Model: "m",
		})
	}))
	defer srv.Close()

	e := New(srv.URL, "", "m", 10)
	chunks := []types.Chunk{{ID: "c1", Content: "x"}}

	embeddings, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	assert.Equal(t, 4, embeddings[0].Dimensions)
}

func TestEmbed_ChunkID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(embedResponse{
			Data:  []embedData{{Index: 0, Embedding: []float64{0.1}}},
			Model: "m",
		})
	}))
	defer srv.Close()

	e := New(srv.URL, "", "m", 10)
	chunks := []types.Chunk{{ID: "my-chunk-id", Content: "x"}}

	embeddings, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	assert.Equal(t, "my-chunk-id", embeddings[0].ChunkID)
}

func TestEmbed_APIClient(t *testing.T) {
	var method, path, contentType, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		contentType = r.Header.Get("Content-Type")
		auth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(embedResponse{
			Data:  []embedData{{Index: 0, Embedding: []float64{0.1}}},
			Model: "m",
		})
	}))
	defer srv.Close()

	e := New(srv.URL, "sk-test-key", "m", 10)
	chunks := []types.Chunk{{ID: "c1", Content: "x"}}

	_, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	assert.Equal(t, "POST", method)
	assert.Equal(t, "/embeddings", path)
	assert.Equal(t, "application/json", contentType)
	assert.Equal(t, "Bearer sk-test-key", auth)
}

func TestEmbed_APIEmptyKey(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(embedResponse{
			Data:  []embedData{{Index: 0, Embedding: []float64{0.1}}},
			Model: "m",
		})
	}))
	defer srv.Close()

	e := New(srv.URL, "", "m", 10)
	chunks := []types.Chunk{{ID: "c1", Content: "x"}}

	_, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	assert.Empty(t, auth, "Authorization should be empty for no API key")
}

func TestEmbed_APIBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer srv.Close()

	e := New(srv.URL, "", "m", 10)
	chunks := []types.Chunk{{ID: "c1", Content: "x"}}

	_, err := e.Embed(context.Background(), chunks)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestEmbed_RateLimit(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := atomic.AddInt32(&callCount, 1)
		if c == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		json.NewEncoder(w).Encode(embedResponse{
			Data:  []embedData{{Index: 0, Embedding: []float64{0.1}}},
			Model: "m",
		})
	}))
	defer srv.Close()

	e := New(srv.URL, "", "m", 10)
	chunks := []types.Chunk{{ID: "c1", Content: "x"}}

	embeddings, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	assert.Len(t, embeddings, 1)
	assert.Equal(t, int32(2), atomic.LoadInt32(&callCount))
}

func TestEmbed_RateLimitExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	e := New(srv.URL, "", "m", 10)
	chunks := []types.Chunk{{ID: "c1", Content: "x"}}

	_, err := e.Embed(context.Background(), chunks)
	require.Error(t, err)
}

func TestEmbed_ResponseMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(embedResponse{
			Data:  []embedData{{Index: 0, Embedding: []float64{0.1}}},
			Model: "m",
		})
	}))
	defer srv.Close()

	e := New(srv.URL, "", "m", 10)
	chunks := []types.Chunk{
		{ID: "c1", Content: "x"},
		{ID: "c2", Content: "y"},
	}

	_, err := e.Embed(context.Background(), chunks)
	require.Error(t, err)
}

func TestEmbed_ModelField(t *testing.T) {
	t.Run("response model used when present", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(embedResponse{
				Data:  []embedData{{Index: 0, Embedding: []float64{0.1}}},
				Model: "response-model",
			})
		}))
		defer srv.Close()

		e := New(srv.URL, "", "configured-model", 10)
		chunks := []types.Chunk{{ID: "c1", Content: "x"}}

		embeddings, err := e.Embed(context.Background(), chunks)
		require.NoError(t, err)
		assert.Equal(t, "response-model", embeddings[0].Model)
	})

	t.Run("falls back to configured model when empty", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(embedResponse{
				Data:  []embedData{{Index: 0, Embedding: []float64{0.1}}},
				Model: "",
			})
		}))
		defer srv.Close()

		e := New(srv.URL, "", "configured-model", 10)
		chunks := []types.Chunk{{ID: "c1", Content: "x"}}

		embeddings, err := e.Embed(context.Background(), chunks)
		require.NoError(t, err)
		assert.Equal(t, "configured-model", embeddings[0].Model)
	})
}

func TestEmbed_EmptyChunkContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(embedResponse{
			Data:  []embedData{{Index: 0, Embedding: []float64{0.1}}},
			Model: "m",
		})
	}))
	defer srv.Close()

	e := New(srv.URL, "", "m", 10)
	chunks := []types.Chunk{{ID: "c1", Content: ""}}

	embeddings, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	assert.Len(t, embeddings, 1)
}

func TestNewEmbedder_Defaults(t *testing.T) {
	e := New("https://api.openai.com/v1", "", "text-embedding-3-small", 20)
	assert.Equal(t, "text-embedding-3-small", e.ModelName())
	assert.Equal(t, 0, e.Dimensions())
}
