package db

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

func Connect(ctx context.Context) (*pgxpool.Pool, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://rag:rag@localhost:5432/rag?sslmode=disable"
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}
	return pool, nil
}

type RiverClient struct {
	Client *river.Client[pgx.Tx]
	Pool   *pgxpool.Pool
}

func NewRiverClient(ctx context.Context, maxAttempts int) (*RiverClient, error) {
	pool, err := Connect(ctx)
	if err != nil {
		return nil, err
	}
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		MaxAttempts: maxAttempts,
	})
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("river client: %w", err)
	}
	return &RiverClient{Client: client, Pool: pool}, nil
}
