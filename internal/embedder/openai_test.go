package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResponse struct {
	Data  []embedData `json:"data"`
	Model string      `json:"model"`
}

type embedData struct {
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func newTestEmbedder(baseURL, apiKey, model string, batchSize int) *embedder {
	e := newOpenAIEmbedder(baseURL, apiKey, model, batchSize)
	e.retryBackoff = time.Millisecond
	return e
}

func TestEmbed_Basic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := embedResponse{
			Data: []embedData{
				{Index: 0, Embedding: []float64{0.1, 0.2, 0.3}},
				{Index: 1, Embedding: []float64{0.4, 0.5, 0.6}},
			},
			Model: "text-embedding-3-small",
		}
		writeJSON(w, resp)
	}))
	defer srv.Close()

	e := newOpenAIEmbedder(srv.URL, "", "text-embedding-3-small", 10)
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
		writeJSON(w, embedResponse{Data: data, Model: "m"})
	}))
	defer srv.Close()

	e := newOpenAIEmbedder(srv.URL, "", "m", 10)
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

	e := newOpenAIEmbedder(srv.URL, "", "m", 10)
	embeddings, err := e.Embed(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, embeddings)
	assert.False(t, called, "API should not be called for empty input")
}

func TestEmbed_ModelName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, embedResponse{
			Data:  []embedData{{Index: 0, Embedding: []float64{0.1}}},
			Model: "custom-model",
		})
	}))
	defer srv.Close()

	e := newOpenAIEmbedder(srv.URL, "", "custom-model", 10)
	chunks := []types.Chunk{{ID: "c1", Content: "x"}}

	embeddings, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	assert.Equal(t, "custom-model", embeddings[0].Model)
}

func TestEmbed_Dimensions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, embedResponse{
			Data:  []embedData{{Index: 0, Embedding: []float64{0.1, 0.2, 0.3, 0.4}}},
			Model: "m",
		})
	}))
	defer srv.Close()

	e := newOpenAIEmbedder(srv.URL, "", "m", 10)
	chunks := []types.Chunk{{ID: "c1", Content: "x"}}

	embeddings, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	assert.Equal(t, 4, embeddings[0].Dimensions)
}

func TestEmbed_ChunkID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, embedResponse{
			Data:  []embedData{{Index: 0, Embedding: []float64{0.1}}},
			Model: "m",
		})
	}))
	defer srv.Close()

	e := newOpenAIEmbedder(srv.URL, "", "m", 10)
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
		writeJSON(w, embedResponse{
			Data:  []embedData{{Index: 0, Embedding: []float64{0.1}}},
			Model: "m",
		})
	}))
	defer srv.Close()

	e := newOpenAIEmbedder(srv.URL, "sk-test-key", "m", 10)
	chunks := []types.Chunk{{ID: "c1", Content: "x"}}

	_, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	assert.Equal(t, "POST", method)
	assert.Equal(t, "/v1/embeddings", path)
	assert.Equal(t, "application/json", contentType)
	assert.Equal(t, "Bearer sk-test-key", auth)
}

func TestEmbed_APIEmptyKey(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		writeJSON(w, embedResponse{
			Data:  []embedData{{Index: 0, Embedding: []float64{0.1}}},
			Model: "m",
		})
	}))
	defer srv.Close()

	e := newOpenAIEmbedder(srv.URL, "", "m", 10)
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

	e := newOpenAIEmbedder(srv.URL, "", "m", 10)
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
		writeJSON(w, embedResponse{
			Data:  []embedData{{Index: 0, Embedding: []float64{0.1}}},
			Model: "m",
		})
	}))
	defer srv.Close()

	e := newTestEmbedder(srv.URL, "", "m", 10)
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

	e := newTestEmbedder(srv.URL, "", "m", 10)
	chunks := []types.Chunk{{ID: "c1", Content: "x"}}

	_, err := e.Embed(context.Background(), chunks)
	require.Error(t, err)
}

func TestEmbed_ResponseMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, embedResponse{
			Data:  []embedData{{Index: 0, Embedding: []float64{0.1}}},
			Model: "m",
		})
	}))
	defer srv.Close()

	e := newOpenAIEmbedder(srv.URL, "", "m", 10)
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
			writeJSON(w, embedResponse{
				Data:  []embedData{{Index: 0, Embedding: []float64{0.1}}},
				Model: "response-model",
			})
		}))
		defer srv.Close()

		e := newOpenAIEmbedder(srv.URL, "", "configured-model", 10)
		chunks := []types.Chunk{{ID: "c1", Content: "x"}}

		embeddings, err := e.Embed(context.Background(), chunks)
		require.NoError(t, err)
		assert.Equal(t, "response-model", embeddings[0].Model)
	})

	t.Run("falls back to configured model when empty", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, embedResponse{
				Data:  []embedData{{Index: 0, Embedding: []float64{0.1}}},
				Model: "",
			})
		}))
		defer srv.Close()

		e := newOpenAIEmbedder(srv.URL, "", "configured-model", 10)
		chunks := []types.Chunk{{ID: "c1", Content: "x"}}

		embeddings, err := e.Embed(context.Background(), chunks)
		require.NoError(t, err)
		assert.Equal(t, "configured-model", embeddings[0].Model)
	})
}

func TestEmbed_EmptyChunkContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, embedResponse{
			Data:  []embedData{{Index: 0, Embedding: []float64{0.1}}},
			Model: "m",
		})
	}))
	defer srv.Close()

	e := newOpenAIEmbedder(srv.URL, "", "m", 10)
	chunks := []types.Chunk{{ID: "c1", Content: ""}}

	embeddings, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	assert.Len(t, embeddings, 1)
}

func TestNewEmbedder_Defaults(t *testing.T) {
	e := newOpenAIEmbedder("https://api.openai.com/v1", "", "text-embedding-3-small", 20)
	assert.Equal(t, "text-embedding-3-small", e.ModelName())
	assert.Equal(t, 0, e.Dimensions())
}

func TestEmbed_RetryBodyPreserved(t *testing.T) {
	var callCount int32
	var bodies [][]byte
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()

		mu.Lock()
		bodies = append(bodies, body)
		count := atomic.AddInt32(&callCount, 1)
		mu.Unlock()

		if count < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeJSON(w, embedResponse{
			Data:  []embedData{{Index: 0, Embedding: []float64{0.1}}},
			Model: "m",
		})
	}))
	defer srv.Close()

	e := newTestEmbedder(srv.URL, "", "m", 10)
	e.retryMaxAttempts = 5
	chunks := []types.Chunk{{ID: "c1", Content: "hello world"}}

	_, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	assert.Equal(t, int32(3), atomic.LoadInt32(&callCount), "expected 3 calls (2 retries + success)")
	require.Len(t, bodies, 3, "expected 3 request bodies")

	for i := 1; i < len(bodies); i++ {
		assert.True(t, bytes.Equal(bodies[0], bodies[i]),
			"request body on attempt %d differs from attempt 0", i+1)
	}
}

func TestEmbed_RetryBodyPreserved_Exhausted(t *testing.T) {
	var bodies [][]byte
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()

		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()

		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	e := newTestEmbedder(srv.URL, "", "m", 10)
	e.retryMaxAttempts = 2
	chunks := []types.Chunk{{ID: "c1", Content: "fail me"}}

	_, err := e.Embed(context.Background(), chunks)
	require.Error(t, err)

	require.Len(t, bodies, 3, "expected 3 attempts (initial + 2 retries)")
	for i := 1; i < len(bodies); i++ {
		assert.True(t, bytes.Equal(bodies[0], bodies[i]),
			"request body on attempt %d differs from attempt 0", i+1)
	}
}
