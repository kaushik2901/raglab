package embedder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

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
	if err != nil {
		t.Fatal(err)
	}
	if len(embeddings) != 2 {
		t.Fatalf("got %d embeddings, want 2", len(embeddings))
	}
	if len(embeddings[0].Vector) != 3 {
		t.Errorf("vector length = %d, want 3", len(embeddings[0].Vector))
	}
	if embeddings[0].Vector[0] != 0.1 {
		t.Errorf("vector[0] = %f, want 0.1", embeddings[0].Vector[0])
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if len(embeddings) != 25 {
		t.Fatalf("got %d embeddings, want 25", len(embeddings))
	}
	if c := atomic.LoadInt32(&callCount); c != 3 {
		t.Errorf("API called %d times, want 3", c)
	}
}

func TestEmbed_EmptyInput(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	e := New(srv.URL, "", "m", 10)
	embeddings, err := e.Embed(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(embeddings) != 0 {
		t.Errorf("got %d embeddings, want 0", len(embeddings))
	}
	if called {
		t.Error("API should not be called for empty input")
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if embeddings[0].Model != "custom-model" {
		t.Errorf("Model = %q, want %q", embeddings[0].Model, "custom-model")
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if embeddings[0].Dimensions != 4 {
		t.Errorf("Dimensions = %d, want 4", embeddings[0].Dimensions)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if embeddings[0].ChunkID != "my-chunk-id" {
		t.Errorf("ChunkID = %q, want %q", embeddings[0].ChunkID, "my-chunk-id")
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if method != "POST" {
		t.Errorf("Method = %q, want POST", method)
	}
	if path != "/embeddings" {
		t.Errorf("Path = %q, want /embeddings", path)
	}
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
	if auth != "Bearer sk-test-key" {
		t.Errorf("Authorization = %q, want Bearer sk-test-key", auth)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if auth != "" {
		t.Errorf("Authorization = %q, want empty for no API key", auth)
	}
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
	if err == nil {
		t.Fatal("expected error for 500 status")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %q, want status 500 mention", err.Error())
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if len(embeddings) != 1 {
		t.Fatalf("got %d embeddings", len(embeddings))
	}
	if c := atomic.LoadInt32(&callCount); c != 2 {
		t.Errorf("API called %d times, want 2 (1 retry)", c)
	}
}

func TestEmbed_RateLimitExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	e := New(srv.URL, "", "m", 10)
	chunks := []types.Chunk{{ID: "c1", Content: "x"}}

	_, err := e.Embed(context.Background(), chunks)
	if err == nil {
		t.Fatal("expected error after rate limit exhaustion")
	}
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
	if err == nil {
		t.Fatal("expected error for response mismatch")
	}
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
		if err != nil {
			t.Fatal(err)
		}
		if embeddings[0].Model != "response-model" {
			t.Errorf("Model = %q, want %q", embeddings[0].Model, "response-model")
		}
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
		if err != nil {
			t.Fatal(err)
		}
		if embeddings[0].Model != "configured-model" {
			t.Errorf("Model = %q, want %q", embeddings[0].Model, "configured-model")
		}
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
	if err != nil {
		t.Fatal(err)
	}
	if len(embeddings) != 1 {
		t.Fatalf("got %d embeddings", len(embeddings))
	}
}

func TestNewEmbedder_Defaults(t *testing.T) {
	e := New("https://api.openai.com/v1", "", "text-embedding-3-small", 20)
	if e.ModelName() != "text-embedding-3-small" {
		t.Errorf("ModelName = %q", e.ModelName())
	}
	if e.Dimensions() != 0 {
		t.Errorf("Dimensions = %d, want 0", e.Dimensions())
	}
}
