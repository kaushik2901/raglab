package store

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/qdrant/go-client/qdrant"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

const upsertBatchSize = 100

type QdrantStore struct {
	client *qdrant.GrpcClient
	apiKey string
}

func NewQdrantStore(apiKey string) *QdrantStore {
	return &QdrantStore{apiKey: apiKey}
}

func (s *QdrantStore) Connect(ctx context.Context, dsn string) error {
	u, err := url.Parse(dsn)
	if err != nil {
		return fmt.Errorf("parse dsn: %w", err)
	}

	host := u.Hostname()
	if host == "" {
		host = "localhost"
	}

	portStr := u.Port()
	port := 6334
	if portStr != "" {
		port, err = strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("parse port: %w", err)
		}
	}

	useTLS := u.Scheme == "https"

	client, err := qdrant.NewGrpcClient(&qdrant.Config{
		Host:   host,
		Port:   port,
		APIKey: s.apiKey,
		UseTLS: useTLS,
	})
	if err != nil {
		return fmt.Errorf("create qdrant client: %w", err)
	}

	s.client = client
	return nil
}

func (s *QdrantStore) EnsureCollection(ctx context.Context, name string, vectorSize int, distance string) error {
	if s.client == nil {
		return fmt.Errorf("not connected")
	}

	existsResp, err := s.client.Collections().CollectionExists(ctx, &qdrant.CollectionExistsRequest{
		CollectionName: name,
	})
	if err != nil {
		return fmt.Errorf("check collection exists: %w", err)
	}

	if existsResp.GetResult().GetExists() {
		return nil
	}

	dist := parseDistance(distance)

	_, err = s.client.Collections().Create(ctx, &qdrant.CreateCollection{
		CollectionName: name,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(vectorSize),
			Distance: dist,
		}),
	})
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}

	return nil
}

func (s *QdrantStore) Store(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error {
	if len(chunks) == 0 {
		return nil
	}

	if s.client == nil {
		return fmt.Errorf("not connected")
	}

	for i := 0; i < len(chunks); i += upsertBatchSize {
		end := i + upsertBatchSize
		if end > len(chunks) {
			end = len(chunks)
		}

		batch := chunks[i:end]
		points := make([]*qdrant.PointStruct, len(batch))
		for j, doc := range batch {
			points[j] = toPoint(doc)
		}

		_, err := s.client.Points().Upsert(ctx, &qdrant.UpsertPoints{
			CollectionName: collectionName,
			Points:         points,
		})
		if err != nil {
			return fmt.Errorf("upsert batch %d-%d: %w", i, end, err)
		}
	}

	return nil
}

func (s *QdrantStore) Close() error {
	if s.client == nil {
		return nil
	}
	return s.client.Close()
}

func toPoint(doc types.DocumentChunk) *qdrant.PointStruct {
	vectors := make([]float32, len(doc.Embedding.Vector))
	for i, v := range doc.Embedding.Vector {
		vectors[i] = float32(v)
	}

	return &qdrant.PointStruct{
		Id: &qdrant.PointId{
			PointIdOptions: &qdrant.PointId_Num{Num: chunkIDToUint64(doc.Chunk.ID)},
		},
		Vectors: qdrant.NewVectors(vectors...),
		Payload: qdrant.NewValueMap(map[string]any{
			"document_path": doc.Chunk.DocumentPath,
			"content":       doc.Chunk.Content,
			"token_count":   doc.Chunk.TokenCount,
			"chunk_index":   doc.Chunk.Index,
			"model":         doc.Embedding.Model,
		}),
	}
}

func chunkIDToUint64(id string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(id); i++ {
		h ^= uint64(id[i])
		h *= 1099511628211
	}
	return h
}

func parseDistance(d string) qdrant.Distance {
	switch d {
	case "Cosine":
		return qdrant.Distance_Cosine
	case "Euclid":
		return qdrant.Distance_Euclid
	case "Dot":
		return qdrant.Distance_Dot
	case "Manhattan":
		return qdrant.Distance_Manhattan
	default:
		return qdrant.Distance_Cosine
	}
}
