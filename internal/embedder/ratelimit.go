package embedder

import (
	"context"

	"golang.org/x/time/rate"

	"github.com/kaushik2901/raglab/internal/types"
)

type RateLimitedEmbedder struct {
	inner  Embedder
	bucket *rate.Limiter
}

func NewRateLimitedEmbedder(inner Embedder, rpm float64) Embedder {
	if rpm <= 0 {
		return inner
	}
	limit := rate.Limit(rpm / 60.0)
	burst := max(int(rpm/60), 1)
	return &RateLimitedEmbedder{
		inner:  inner,
		bucket: rate.NewLimiter(limit, burst),
	}
}

func (r *RateLimitedEmbedder) Embed(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
	if err := r.bucket.Wait(ctx); err != nil {
		return nil, err
	}
	return r.inner.Embed(ctx, chunks)
}

func (r *RateLimitedEmbedder) Dimensions() int {
	return r.inner.Dimensions()
}

func (r *RateLimitedEmbedder) ModelName() string {
	return r.inner.ModelName()
}
