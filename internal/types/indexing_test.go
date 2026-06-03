package types

import "testing"

func TestChunkCreation(t *testing.T) {
	chunk := Chunk{
		ID:           "abc-123",
		DocumentPath: "docs/foo.md",
		Content:      "some content",
		Metadata:     map[string]string{"heading": "Intro"},
		TokenCount:   42,
		Index:        0,
	}
	if chunk.ID != "abc-123" {
		t.Errorf("ID = %q, want %q", chunk.ID, "abc-123")
	}
	if chunk.DocumentPath != "docs/foo.md" {
		t.Errorf("DocumentPath = %q, want %q", chunk.DocumentPath, "docs/foo.md")
	}
	if chunk.Content != "some content" {
		t.Errorf("Content = %q, want %q", chunk.Content, "some content")
	}
	if v := chunk.Metadata["heading"]; v != "Intro" {
		t.Errorf(`Metadata["heading"] = %q, want %q`, v, "Intro")
	}
	if chunk.TokenCount != 42 {
		t.Errorf("TokenCount = %d, want %d", chunk.TokenCount, 42)
	}
	if chunk.Index != 0 {
		t.Errorf("Index = %d, want %d", chunk.Index, 0)
	}
}

func TestChunkZeroValue(t *testing.T) {
	var chunk Chunk
	if chunk.ID != "" {
		t.Errorf("zero value ID = %q, want %q", chunk.ID, "")
	}
	if chunk.DocumentPath != "" {
		t.Errorf("zero value DocumentPath = %q, want %q", chunk.DocumentPath, "")
	}
	if chunk.Content != "" {
		t.Errorf("zero value Content = %q, want %q", chunk.Content, "")
	}
	if chunk.Metadata != nil {
		t.Errorf("zero value Metadata = %v, want nil", chunk.Metadata)
	}
	if chunk.TokenCount != 0 {
		t.Errorf("zero value TokenCount = %d, want %d", chunk.TokenCount, 0)
	}
	if chunk.Index != 0 {
		t.Errorf("zero value Index = %d, want %d", chunk.Index, 0)
	}
}

func TestChunkMetadataNil(t *testing.T) {
	chunk := Chunk{ID: "test"}
	if chunk.Metadata != nil {
		t.Error("default Metadata should be nil, not empty map")
	}
}

func TestEmbeddingCreation(t *testing.T) {
	emb := Embedding{
		ChunkID:    "abc-123",
		Vector:     []float64{0.1, 0.2, 0.3},
		Model:      "text-embedding-3-small",
		Dimensions: 3,
	}
	if emb.ChunkID != "abc-123" {
		t.Errorf("ChunkID = %q, want %q", emb.ChunkID, "abc-123")
	}
	if len(emb.Vector) != 3 || emb.Vector[0] != 0.1 || emb.Vector[1] != 0.2 || emb.Vector[2] != 0.3 {
		t.Errorf("Vector = %v, want [0.1 0.2 0.3]", emb.Vector)
	}
	if emb.Model != "text-embedding-3-small" {
		t.Errorf("Model = %q, want %q", emb.Model, "text-embedding-3-small")
	}
	if emb.Dimensions != 3 {
		t.Errorf("Dimensions = %d, want %d", emb.Dimensions, 3)
	}
}

func TestEmbeddingZeroValue(t *testing.T) {
	var emb Embedding
	if emb.ChunkID != "" {
		t.Errorf("zero value ChunkID = %q, want %q", emb.ChunkID, "")
	}
	if emb.Vector != nil {
		t.Errorf("zero value Vector = %v, want nil", emb.Vector)
	}
	if emb.Model != "" {
		t.Errorf("zero value Model = %q, want %q", emb.Model, "")
	}
	if emb.Dimensions != 0 {
		t.Errorf("zero value Dimensions = %d, want %d", emb.Dimensions, 0)
	}
}

func TestEmbeddingVectorType(t *testing.T) {
	vec := []float64{}
	vec = append(vec, 1.5, 2.5, 3.5)
	emb := Embedding{Vector: vec}
	if len(emb.Vector) != 3 {
		t.Errorf("Vector length = %d, want %d", len(emb.Vector), 3)
	}
	if emb.Vector[0] != 1.5 {
		t.Errorf("Vector[0] = %f, want %f", emb.Vector[0], 1.5)
	}
}

func TestDocumentChunkCreation(t *testing.T) {
	chunk := Chunk{ID: "chunk-1", Content: "hello", Index: 0}
	emb := Embedding{ChunkID: "chunk-1", Vector: []float64{0.1}, Model: "m1"}
	dc := DocumentChunk{Chunk: chunk, Embedding: emb}
	if dc.Chunk.ID != "chunk-1" {
		t.Errorf("Chunk.ID = %q, want %q", dc.Chunk.ID, "chunk-1")
	}
	if dc.Embedding.ChunkID != "chunk-1" {
		t.Errorf("Embedding.ChunkID = %q, want %q", dc.Embedding.ChunkID, "chunk-1")
	}
	if dc.Embedding.Model != "m1" {
		t.Errorf("Embedding.Model = %q, want %q", dc.Embedding.Model, "m1")
	}
}

func TestDocumentChunkRoundTrip(t *testing.T) {
	chunk := Chunk{
		ID:           "doc1-chunk-0000",
		DocumentPath: "docs/page.md",
		Content:      "some text",
		TokenCount:   8,
		Index:        0,
	}
	emb := Embedding{
		ChunkID:    "doc1-chunk-0000",
		Vector:     []float64{0.01, 0.02},
		Model:      "text-embedding-3-small",
		Dimensions: 2,
	}
	dc := DocumentChunk{Chunk: chunk, Embedding: emb}
	if dc.Chunk.ID != dc.Embedding.ChunkID {
		t.Errorf("Chunk.ID = %q != Embedding.ChunkID = %q", dc.Chunk.ID, dc.Embedding.ChunkID)
	}
	if dc.Chunk.DocumentPath != "docs/page.md" {
		t.Errorf("DocumentPath = %q", dc.Chunk.DocumentPath)
	}
	if dc.Embedding.Dimensions != 2 {
		t.Errorf("Dimensions = %d", dc.Embedding.Dimensions)
	}
}
