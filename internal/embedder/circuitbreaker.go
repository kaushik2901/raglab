package embedder

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/sony/gobreaker"

	"github.com/kaushik2901/raglab/internal/types"
)

type CircuitBreakerEmbedder struct {
	inner   Embedder
	breaker *gobreaker.CircuitBreaker
}

func NewCircuitBreakerEmbedder(inner Embedder) Embedder {
	return &CircuitBreakerEmbedder{
		inner: inner,
		breaker: gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:        "embedder",
			MaxRequests: 1,
			Interval:    10 * time.Second,
			Timeout:     30 * time.Second,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures > 5
			},
			OnStateChange: func(name string, from, to gobreaker.State) {
				slog.Warn("circuit breaker state change",
					"name", name, "from", from.String(), "to", to.String())
			},
		}),
	}
}

func (c *CircuitBreakerEmbedder) Embed(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
	result, err := c.breaker.Execute(func() (any, error) {
		return c.inner.Embed(ctx, chunks)
	})
	if err != nil {
		return nil, err
	}
	embeddings, ok := result.([]types.Embedding)
	if !ok {
		return nil, fmt.Errorf("circuit breaker: unexpected result type %T", result)
	}
	return embeddings, nil
}

func (c *CircuitBreakerEmbedder) Dimensions() int {
	return c.inner.Dimensions()
}

func (c *CircuitBreakerEmbedder) ModelName() string {
	return c.inner.ModelName()
}
