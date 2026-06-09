package store

import (
	"context"
	"crypto/sha1"
	"fmt"
	"net/url"
	"strconv"

	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

const upsertBatchSize = 100

type QdrantStore struct {
	client  *qdrant.GrpcClient
	apiKey  string
	lastDSN string
}

func NewQdrantStore(apiKey string) *QdrantStore {
	return &QdrantStore{apiKey: apiKey}
}

func (s *QdrantStore) Connect(ctx context.Context, dsn string) error {
	s.lastDSN = dsn

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
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			if err := s.reconnect(ctx); err != nil {
				return fmt.Errorf("reconnect failed: %w", err)
			}
		}
		err := s.ensureCollectionOnce(ctx, name, vectorSize, distance)
		if err == nil {
			return nil
		}
		if isConnError(err) {
			continue
		}
		return err
	}
	return fmt.Errorf("ensure collection failed after 3 attempts")
}

func (s *QdrantStore) ensureCollectionOnce(ctx context.Context, name string, vectorSize int, distance string) error {
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
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			if err := s.reconnect(ctx); err != nil {
				return fmt.Errorf("reconnect failed: %w", err)
			}
		}
		err := s.storeOnce(ctx, collectionName, chunks)
		if err == nil {
			return nil
		}
		if isConnError(err) {
			continue
		}
		return err
	}
	return fmt.Errorf("store failed after 3 attempts")
}

func (s *QdrantStore) storeOnce(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error {
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

func (s *QdrantStore) Search(ctx context.Context, collectionName string, queryVector []float32, topK int) ([]types.SearchResult, error) {
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			if err := s.reconnect(ctx); err != nil {
				return nil, fmt.Errorf("reconnect failed: %w", err)
			}
		}
		results, err := s.searchOnce(ctx, collectionName, queryVector, topK)
		if err == nil {
			return results, nil
		}
		if isConnError(err) {
			continue
		}
		return nil, err
	}
	return nil, fmt.Errorf("search failed after 3 attempts")
}

func (s *QdrantStore) searchOnce(ctx context.Context, collectionName string, queryVector []float32, topK int) ([]types.SearchResult, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not connected")
	}

	resp, err := s.client.Points().Search(ctx, &qdrant.SearchPoints{
		CollectionName: collectionName,
		Vector:         queryVector,
		Limit:          uint64(topK),
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	results := make([]types.SearchResult, 0, len(resp.GetResult()))
	for _, p := range resp.GetResult() {
		r := types.SearchResult{
			Score: p.GetScore(),
		}
		if payload := p.GetPayload(); payload != nil {
			if v, ok := payload["content"]; ok {
				r.Content = v.GetStringValue()
			}
			if v, ok := payload["document_path"]; ok {
				r.DocumentPath = v.GetStringValue()
			}
			if v, ok := payload["token_count"]; ok {
				r.TokenCount = int(v.GetIntegerValue())
			}
			if v, ok := payload["chunk_index"]; ok {
				r.ChunkIndex = int(v.GetIntegerValue())
			}
		}
		results = append(results, r)
	}

	return results, nil
}

func (s *QdrantStore) HealthCheck(ctx context.Context) error {
	_, err := s.client.Collections().CollectionExists(ctx, &qdrant.CollectionExistsRequest{
		CollectionName: "_health_check",
	})
	return err
}

func (s *QdrantStore) Close() error {
	if s.client == nil {
		return nil
	}
	return s.client.Close()
}

func (s *QdrantStore) reconnect(ctx context.Context) error {
	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
	if s.lastDSN == "" {
		return fmt.Errorf("no last DSN — Connect was never called")
	}
	return s.Connect(ctx, s.lastDSN)
}

func isConnError(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	switch st.Code() {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled:
		return true
	}
	return false
}

func toPoint(doc types.DocumentChunk) *qdrant.PointStruct {
	vectors := make([]float32, len(doc.Embedding.Vector))
	for i, v := range doc.Embedding.Vector {
		vectors[i] = float32(v)
	}

	return &qdrant.PointStruct{
		Id: &qdrant.PointId{
			PointIdOptions: &qdrant.PointId_Uuid{Uuid: chunkIDToUUID(doc.Chunk.ID)},
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

// dnsNamespace is the UUID for the DNS namespace (RFC 4122).
var dnsNamespace = []byte{
	0x6b, 0xa7, 0xb8, 0x10, 0x9d, 0xad, 0x11, 0xd1,
	0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8,
}

// chunkIDToUUID generates a deterministic UUID v5 from a chunk ID string
// using the DNS namespace. This guarantees collision-free point IDs at any
// corpus scale.
func chunkIDToUUID(id string) string {
	h := sha1.New()
	h.Write(dnsNamespace)
	h.Write([]byte(id))
	sum := h.Sum(nil)[:16]
	sum[6] = (sum[6] & 0x0f) | 0x50 // set version 5
	sum[8] = (sum[8] & 0x3f) | 0x80 // set RFC 4122 variant
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
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
