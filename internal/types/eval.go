package types

type EvalQuestion struct {
	ID                 string   `json:"id"`
	Category           string   `json:"category"`
	Difficulty         string   `json:"difficulty"`
	Question           string   `json:"question"`
	Answer             string   `json:"answer"`
	SourcePaths        []string `json:"source_paths"`
	Keywords           []string `json:"keywords,omitempty"`
	ExpectedChunkTopics []string `json:"expected_chunk_topics,omitempty"`
}

type EvalDataset struct {
	Meta      EvalDatasetMeta `json:"meta"`
	Questions []EvalQuestion  `json:"questions"`
}

type EvalDatasetMeta struct {
	Created     string `json:"created"`
	Version     int    `json:"version"`
	Description string `json:"description"`
}

type EvalStrategyConfig struct {
	Tag           string
	IndexTag      string
	QueryStrategy string
	TopK          int
	LLMModel      string
}

type RetrievalResult struct {
	QuestionID    string
	Question      string
	ExpectedPaths []string
	RetrievedPaths []string
	Scores        []float64
	Hit           map[int]bool
	RankFirst     int
	Answer        string
	PromptTokens  int
	CompletionTokens int
	LatencyMs     int64
}

type AggregateMetrics struct {
	HitRate    map[int]float64
	MRR        float64
	NDCG       map[int]float64
	Precision  map[int]float64
	Recall     map[int]float64
}

type EvalReport struct {
	RunID        string
	Strategy     EvalStrategyConfig
	Questions    int
	Aggregate    AggregateMetrics
	PerQuestion  []RetrievalResult
}

type EvalRun struct {
	ID         string
	WorkflowID string
	Tag        string
	Strategy   map[string]any
	Metrics    map[string]any
}
