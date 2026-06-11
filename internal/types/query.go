package types

type SearchResult struct {
	ChunkID      string
	DocumentPath string
	Content      string
	Score        float32
	TokenCount   int
	ChunkIndex   int
	Metadata     map[string]string
}
