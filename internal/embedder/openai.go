package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type embedder struct {
	baseURL          string
	apiKey           string
	model            string
	batchSize        int
	client           *http.Client
	retryMaxAttempts int
	retryBackoff     time.Duration
	dimensions       atomic.Int32
}

func newOpenAIEmbedder(baseURL, apiKey, model string, batchSize int) *embedder {
	config.WarnOnInsecure(baseURL, apiKey, "embedder")
	return &embedder{
		baseURL: config.NormalizeBaseURL(baseURL),
		apiKey:           apiKey,
		model:            model,
		batchSize:        batchSize,
		client:           &http.Client{Timeout: 60 * time.Second},
		retryMaxAttempts: 5,
		retryBackoff:     200 * time.Millisecond,
	}
}

func (e *embedder) Embed(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
	if len(chunks) == 0 {
		return nil, nil
	}

	var embeddings []types.Embedding
	for i := 0; i < len(chunks); i += e.batchSize {
		end := i + e.batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[i:end]

		batchEmbeddings, err := e.embedBatch(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("embed batch %d-%d: %w", i, end, err)
		}
		embeddings = append(embeddings, batchEmbeddings...)
	}

	return embeddings, nil
}

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

func (e *embedder) embedBatch(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
	inputs := make([]string, len(chunks))
	for i, ch := range chunks {
		inputs[i] = ch.Content
	}

	reqBody := embedRequest{
		Model: e.model,
		Input: inputs,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	var resp embedResponse
	if err := e.doWithRetry(ctx, body, &resp); err != nil {
		return nil, err
	}

	if len(resp.Data) != len(chunks) {
		return nil, fmt.Errorf("response has %d embeddings, expected %d", len(resp.Data), len(chunks))
	}

	modelName := resp.Model
	if modelName == "" {
		modelName = e.model
	}

	embeddings := make([]types.Embedding, len(chunks))
	for i, d := range resp.Data {
		e.dimensions.CompareAndSwap(0, int32(len(d.Embedding)))
		embeddings[i] = types.Embedding{
			ChunkID:    chunks[i].ID,
			Vector:     d.Embedding,
			Model:      modelName,
			Dimensions: len(d.Embedding),
		}
	}

	return embeddings, nil
}

func (e *embedder) doWithRetry(ctx context.Context, body []byte, respVal interface{}) error {
	for attempt := 0; attempt <= e.retryMaxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/v1/embeddings", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if e.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+e.apiKey)
		}

		backoff := e.retryBackoff * (1 << attempt)
		resp, err := e.client.Do(req)
		if err != nil {
			return fmt.Errorf("http request: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < e.retryMaxAttempts {
			retryAfter := config.ParseRetryAfter(resp.Header.Get("Retry-After"))
			if retryAfter > backoff {
				backoff = retryAfter
			}
			jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
			resp.Body.Close()
			time.Sleep(backoff + jitter)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(bodyBytes))
		}

		if err := json.NewDecoder(resp.Body).Decode(respVal); err != nil {
			resp.Body.Close()
			return fmt.Errorf("decode response: %w", err)
		}
		resp.Body.Close()
		return nil
	}

	return fmt.Errorf("rate limit exceeded after %d retries", e.retryMaxAttempts)
}

func (e *embedder) Dimensions() int {
	return int(e.dimensions.Load())
}

func (e *embedder) ModelName() string {
	return e.model
}
