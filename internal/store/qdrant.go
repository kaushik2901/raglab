package store

import (
	"context"
	"crypto/sha1"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kaushik2901/raglab/internal/types"
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
	return retryWithBackoff(ctx, 3, func(ctx context.Context) error {
		return s.ensureCollectionOnce(ctx, name, vectorSize, distance)
	}, func() error { return s.reconnect(ctx) })
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
	return retryWithBackoff(ctx, 3, func(ctx context.Context) error {
		return s.storeOnce(ctx, collectionName, chunks)
	}, func() error { return s.reconnect(ctx) })
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
	var results []types.SearchResult
	err := retryWithBackoff(ctx, 3, func(ctx context.Context) error {
		var err error
		results, err = s.searchOnce(ctx, collectionName, queryVector, topK)
		return err
	}, func() error { return s.reconnect(ctx) })
	return results, err
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
		WithVectors:    qdrant.NewWithVectors(true),
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
			r.Metadata = make(map[string]string)
			for k, v := range payload {
				switch k {
				case "content", "document_path", "token_count", "chunk_index", "model":
					continue
				default:
					r.Metadata[k] = v.GetStringValue()
				}
			}
		}
		if vectors := p.GetVectors(); vectors != nil {
			if v := vectors.GetVector(); v != nil {
				r.Vector = v.GetData()
			}
		}
		results = append(results, r)
	}

	return results, nil
}

func (s *QdrantStore) ListCollections(ctx context.Context) ([]CollectionInfo, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not connected")
	}
	resp, err := s.client.Collections().List(ctx, &qdrant.ListCollectionsRequest{})
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	collections := resp.GetCollections()
	result := make([]CollectionInfo, 0, len(collections))
	for _, c := range collections {
		info, err := s.GetCollection(ctx, c.GetName())
		if err != nil {
			result = append(result, CollectionInfo{Name: c.GetName()})
			continue
		}
		result = append(result, *info)
	}
	return result, nil
}

func (s *QdrantStore) GetCollection(ctx context.Context, name string) (*CollectionInfo, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not connected")
	}
	resp, err := s.client.Collections().Get(ctx, &qdrant.GetCollectionInfoRequest{
		CollectionName: name,
	})
	if err != nil {
		st, _ := status.FromError(err)
		if st.Code() == codes.NotFound {
			return nil, fmt.Errorf("%w: %s", ErrCollectionNotFound, name)
		}
		return nil, fmt.Errorf("get collection %s: %w", name, err)
	}
	info := &CollectionInfo{Name: name}
	if result := resp.GetResult(); result != nil {
		if config := result.GetConfig(); config != nil {
			if params := config.GetParams(); params != nil {
				if vc := params.GetVectorsConfig(); vc != nil {
					if vp := vc.GetParams(); vp != nil {
						info.VectorSize = vp.GetSize()
						info.Distance = distanceString(vp.GetDistance())
					}
				}
			}
		}
		info.VectorCount = result.GetPointsCount()
	}
	return info, nil
}

func (s *QdrantStore) DeleteCollection(ctx context.Context, name string) error {
	if s.client == nil {
		return fmt.Errorf("not connected")
	}
	_, err := s.client.Collections().Delete(ctx, &qdrant.DeleteCollection{
		CollectionName: name,
	})
	if err != nil {
		st, _ := status.FromError(err)
		if st.Code() == codes.NotFound {
			return fmt.Errorf("%w: %s", ErrCollectionNotFound, name)
		}
		return fmt.Errorf("delete collection %s: %w", name, err)
	}
	return nil
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

	payload := map[string]any{
		"document_path": doc.Chunk.DocumentPath,
		"content":       doc.Chunk.Content,
		"token_count":   doc.Chunk.TokenCount,
		"chunk_index":   doc.Chunk.Index,
		"model":         doc.Embedding.Model,
	}
	for k, v := range doc.Chunk.Metadata {
		payload[k] = v
	}

	return &qdrant.PointStruct{
		Id: &qdrant.PointId{
			PointIdOptions: &qdrant.PointId_Uuid{Uuid: chunkIDToUUID(doc.Chunk.ID)},
		},
		Vectors: qdrant.NewVectors(vectors...),
		Payload: qdrant.NewValueMap(payload),
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

func retryWithBackoff(ctx context.Context, maxAttempts int, op func(context.Context) error, reconnect func() error) error {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 100 * time.Millisecond
	b.Multiplier = 2.0
	b.MaxInterval = 5 * time.Second
	b.MaxElapsedTime = 0
	b.RandomizationFactor = 0.5

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			next := b.NextBackOff()
			if next == backoff.Stop {
				return fmt.Errorf("retry stopped after %d attempts", attempt)
			}
			select {
			case <-time.After(next):
			case <-ctx.Done():
				return ctx.Err()
			}
			if err := reconnect(); err != nil {
				return fmt.Errorf("reconnect failed: %w", err)
			}
		}
		err := op(ctx)
		if err == nil {
			return nil
		}
		if !isConnError(err) {
			return err
		}
	}
	return fmt.Errorf("operation failed after %d attempts", maxAttempts)
}

func distanceString(d qdrant.Distance) string {
	switch d {
	case qdrant.Distance_Cosine:
		return "Cosine"
	case qdrant.Distance_Euclid:
		return "Euclid"
	case qdrant.Distance_Dot:
		return "Dot"
	case qdrant.Distance_Manhattan:
		return "Manhattan"
	default:
		return "Unknown"
	}
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
