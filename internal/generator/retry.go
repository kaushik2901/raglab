package generator

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
)

type RetryGenerator struct {
	inner   Generator
	backOff backoff.BackOff
}

func NewRetryGenerator(inner Generator) Generator {
	return &RetryGenerator{inner: inner, backOff: newExponentialBackoff()}
}

func NewRetryGeneratorWithBackOff(inner Generator, b backoff.BackOff) Generator {
	return &RetryGenerator{inner: inner, backOff: b}
}

func (r *RetryGenerator) Generate(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	b := backoff.WithContext(r.backOff, ctx)

	operation := func() (*openai.ChatCompletion, error) {
		result, err := r.inner.Generate(ctx, params)
		if err == nil {
			return result, nil
		}
		if isRetryable(err) {
			return nil, err
		}
		return nil, backoff.Permanent(err)
	}

	return backoff.RetryWithData(operation, b)
}

func (r *RetryGenerator) GenerateStream(ctx context.Context, params openai.ChatCompletionNewParams, cb StreamCallback) (*openai.ChatCompletion, error) {
	return r.inner.GenerateStream(ctx, params, cb)
}

func (r *RetryGenerator) ModelName() string {
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

	slog.Debug("non-retryable generator error", "err", err)
	return false
}
