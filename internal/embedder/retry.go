package embedder

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/openai/openai-go"

	"github.com/kaushik2901/raglab/internal/types"
)

type RetryEmbedder struct {
	inner   Embedder
	backOff backoff.BackOff
}

func NewRetryEmbedder(inner Embedder) Embedder {
	return &RetryEmbedder{inner: inner, backOff: newExponentialBackoff()}
}

func NewRetryEmbedderWithBackOff(inner Embedder, b backoff.BackOff) Embedder {
	return &RetryEmbedder{inner: inner, backOff: b}
}

func (r *RetryEmbedder) Embed(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
	b := backoff.WithContext(r.backOff, ctx)

	operation := func() ([]types.Embedding, error) {
		result, err := r.inner.Embed(ctx, chunks)
		if err == nil {
			return result, nil
		}
		if isRetryable(err) {
			return nil, err
		}
		return nil, backoff.Permanent(err)
	}

	result, err := backoff.RetryWithData(operation, b)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *RetryEmbedder) Dimensions() int {
	return r.inner.Dimensions()
}

func (r *RetryEmbedder) ModelName() string {
	return r.inner.ModelName()
}

func newExponentialBackoff() *backoff.ExponentialBackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 200 * time.Millisecond
	b.Multiplier = 2.0
	b.MaxInterval = 10 * time.Second
	b.MaxElapsedTime = 30 * time.Second
	b.RandomizationFactor = 0.5
	return b
}

func isRetryable(err error) bool {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusTooManyRequests,
			http.StatusInternalServerError,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout:
			return true
		}
	}

	if errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, io.EOF) {
		return true
	}

	if errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ENETUNREACH) ||
		errors.Is(err, syscall.ETIMEDOUT) {
		return true
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	slog.Debug("non-retryable error", "err", err)
	return false
}
