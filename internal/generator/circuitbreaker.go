package generator

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/openai/openai-go"
	"github.com/sony/gobreaker"
)

type CircuitBreakerGenerator struct {
	inner           Generator
	generateBreaker *gobreaker.CircuitBreaker
	streamBreaker   *gobreaker.CircuitBreaker
}

func NewCircuitBreakerGenerator(inner Generator) Generator {
	breaker := func(name string) *gobreaker.CircuitBreaker {
		return gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:        name,
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
		})
	}
	return &CircuitBreakerGenerator{
		inner:           inner,
		generateBreaker: breaker("generator-generate"),
		streamBreaker:   breaker("generator-stream"),
	}
}

func (c *CircuitBreakerGenerator) Generate(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	result, err := c.generateBreaker.Execute(func() (any, error) {
		return c.inner.Generate(ctx, params)
	})
	if err != nil {
		return nil, err
	}
	completion, ok := result.(*openai.ChatCompletion)
	if !ok {
		return nil, fmt.Errorf("circuit breaker: unexpected result type %T", result)
	}
	return completion, nil
}

func (c *CircuitBreakerGenerator) GenerateStream(ctx context.Context, params openai.ChatCompletionNewParams, cb StreamCallback) (*openai.ChatCompletion, error) {
	result, err := c.streamBreaker.Execute(func() (any, error) {
		return c.inner.GenerateStream(ctx, params, cb)
	})
	if err != nil {
		return nil, err
	}
	completion, ok := result.(*openai.ChatCompletion)
	if !ok {
		return nil, fmt.Errorf("circuit breaker: unexpected result type %T", result)
	}
	return completion, nil
}

func (c *CircuitBreakerGenerator) ModelName() string {
	return c.inner.ModelName()
}
