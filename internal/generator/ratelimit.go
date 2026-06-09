package generator

import (
	"context"

	"github.com/openai/openai-go"
	"golang.org/x/time/rate"
)

type RateLimitedGenerator struct {
	inner  Generator
	bucket *rate.Limiter
}

func NewRateLimitedGenerator(inner Generator, rpm float64) Generator {
	if rpm <= 0 {
		return inner
	}
	limit := rate.Limit(rpm / 60.0)
	burst := max(int(rpm/60), 1)
	return &RateLimitedGenerator{
		inner:  inner,
		bucket: rate.NewLimiter(limit, burst),
	}
}

func (r *RateLimitedGenerator) Generate(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	if err := r.bucket.Wait(ctx); err != nil {
		return nil, err
	}
	return r.inner.Generate(ctx, params)
}

func (r *RateLimitedGenerator) GenerateStream(ctx context.Context, params openai.ChatCompletionNewParams, cb StreamCallback) (*openai.ChatCompletion, error) {
	if err := r.bucket.Wait(ctx); err != nil {
		return nil, err
	}
	return r.inner.GenerateStream(ctx, params, cb)
}

func (r *RateLimitedGenerator) ModelName() string {
	return r.inner.ModelName()
}
