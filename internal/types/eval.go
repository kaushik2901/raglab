package types

type RelevanceJudgment struct {
	DocumentID string `json:"document_id"`
	Grade      int    `json:"grade"` // 0=irrelevant .. 3=highly relevant
}

type EvalQuestion struct {
	ID             string              `json:"id"`
	Category       string              `json:"category,omitempty"`
	Difficulty     string              `json:"difficulty,omitempty"`
	Question       string              `json:"question"`
	Relevance      []RelevanceJudgment `json:"relevance"`
	ExpectedAnswer string              `json:"expected_answer"`
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
	QuestionID       string
	Question         string
	ExpectedAnswer   string
	Relevance        []RelevanceJudgment
	ExpectedPaths    []string
	RetrievedPaths   []string
	Scores           []float64
	Hit              map[int]bool
	RankFirst        int
	NDCGGraded       float64
	Answer           string
	AnswerScore      float64
	PromptTokens     int
	CompletionTokens int
	LatencyMs        int64
}

type AggregateMetrics struct {
	HitRate        map[int]float64
	MRR            float64
	NDCG           map[int]float64
	NDCGGraded     map[int]float64
	Precision      map[int]float64
	Recall         map[int]float64
	AvgAnswerScore float64
}

type EvalReport struct {
	RunID       string
	Strategy    EvalStrategyConfig
	Questions   int
	Aggregate   AggregateMetrics
	PerQuestion []RetrievalResult
}

type EvalRun struct {
	ID         string
	WorkflowID string
	Tag        string
	Strategy   map[string]any
	Metrics    map[string]any
}
