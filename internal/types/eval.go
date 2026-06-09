package types

type RelevanceJudgment struct {
	DocumentPath string `json:"document_path"`
	Grade        int    `json:"grade"` // 0=irrelevant .. 3=highly relevant
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
	Questions []EvalQuestion `json:"questions"`
}

type RetrievalResult struct {
	QuestionID       string
	Question         string
	Category         string
	Difficulty       string
	ExpectedAnswer   string
	Relevance        []RelevanceJudgment
	ExpectedPaths    []string
	RetrievedPaths   []string
	Scores           []float64
	Hit              map[int]bool
	RankFirst        int
	NDCGGraded       float64
	NDCGGradedK      int  // k used to compute NDCGGraded
	Failed           bool // true if generate/judge failed for this question
	Answer           string
	AnswerScore      float64
	PromptTokens     int
	CompletionTokens int
	LatencyMs        int64
}

type AggregateMetrics struct {
	HitRate               map[int]float64
	MRR                   float64
	NDCG                  map[int]float64
	NDCGGraded            map[int]float64
	Precision             map[int]float64
	Recall                map[int]float64
	AvgAnswerScore        float64
	AvgLatencyMs          float64
	TotalLatencyMs        int64
	AvgPromptTokens       float64
	AvgCompletionTokens   float64
	TotalPromptTokens     int
	TotalCompletionTokens int
}
