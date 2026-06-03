package types

type Chunk struct {
	ID           string
	DocumentPath string
	Content      string
	Metadata     map[string]string
	TokenCount   int
	Index        int
}

type Embedding struct {
	ChunkID    string
	Vector     []float64
	Model      string
	Dimensions int
}

type DocumentChunk struct {
	Chunk     Chunk
	Embedding Embedding
}
