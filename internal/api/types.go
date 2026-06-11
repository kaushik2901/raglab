package api

type PreprocessRequest struct {
	RepoURL     string   `json:"repo_url"`
	Tag         string   `json:"tag"`
	BaseURL     string   `json:"base_url"`
	IncludeDirs []string `json:"include_dirs,omitempty"`
}

func (r PreprocessRequest) Validate() error {
	if r.RepoURL == "" {
		return &validationError{"repo_url is required"}
	}
	if r.Tag == "" {
		return &validationError{"tag is required"}
	}
	return nil
}

type IndexRequest struct {
	InputTag          string `json:"input_tag"`
	Tag               string `json:"tag"`
	ParserStrategy    string `json:"parser_strategy"`
	ChunkStrategy     string `json:"chunk_strategy"`
	ChunkSize         int    `json:"chunk_size"`
	ChunkOverlap      int    `json:"chunk_overlap"`
	EmbeddingProvider string `json:"embedding_provider"`
	EmbeddingModel    string `json:"embedding_model"`
	BatchSize         int    `json:"batch_size"`
	IndexConcurrency  int    `json:"index_concurrency"`
	DocTimeout        string `json:"doc_timeout"`
}

func (r IndexRequest) Validate() error {
	if r.InputTag == "" {
		return &validationError{"input_tag is required"}
	}
	if r.Tag == "" {
		return &validationError{"tag is required"}
	}
	if r.ParserStrategy == "" {
		return &validationError{"parser_strategy is required"}
	}
	if r.ChunkStrategy == "" {
		return &validationError{"chunk_strategy is required"}
	}
	if r.ChunkSize <= 0 {
		return &validationError{"chunk_size must be positive"}
	}
	if r.ChunkOverlap < 0 {
		return &validationError{"chunk_overlap must be non-negative"}
	}
	if r.EmbeddingProvider == "" {
		return &validationError{"embedding_provider is required"}
	}
	if r.EmbeddingModel == "" {
		return &validationError{"embedding_model is required"}
	}
	if r.BatchSize <= 0 {
		return &validationError{"batch_size must be positive"}
	}
	if r.IndexConcurrency <= 0 {
		return &validationError{"index_concurrency must be positive"}
	}
	if r.DocTimeout == "" {
		return &validationError{"doc_timeout is required"}
	}
	return nil
}

type EvalRequest struct {
	IndexTag          string `json:"index_tag"`
	Tag               string `json:"tag"`
	QueryStrategy     string `json:"query_strategy"`
	DatasetPath       string `json:"dataset_path"`
	Ks                []int  `json:"ks"`
	LLMProvider       string `json:"llm_provider"`
	LLMModel          string `json:"llm_model"`
	EmbeddingProvider string `json:"embedding_provider"`
	EmbeddingModel    string `json:"embedding_model"`
	JudgeProvider     string `json:"judge_provider"`
	JudgeModel        string `json:"judge_model"`
	BatchSize         int    `json:"batch_size"`
	Workers           int    `json:"workers"`
}

func (r EvalRequest) Validate() error {
	if r.IndexTag == "" {
		return &validationError{"index_tag is required"}
	}
	if r.Tag == "" {
		return &validationError{"tag is required"}
	}
	if r.QueryStrategy == "" {
		return &validationError{"query_strategy is required"}
	}
	if r.DatasetPath == "" {
		return &validationError{"dataset_path is required"}
	}
	if len(r.Ks) == 0 {
		return &validationError{"ks is required"}
	}
	if r.LLMProvider == "" {
		return &validationError{"llm_provider is required"}
	}
	if r.LLMModel == "" {
		return &validationError{"llm_model is required"}
	}
	if r.EmbeddingProvider == "" {
		return &validationError{"embedding_provider is required"}
	}
	if r.EmbeddingModel == "" {
		return &validationError{"embedding_model is required"}
	}
	if r.JudgeProvider == "" {
		return &validationError{"judge_provider is required"}
	}
	if r.JudgeModel == "" {
		return &validationError{"judge_model is required"}
	}
	if r.BatchSize <= 0 {
		return &validationError{"batch_size must be positive"}
	}
	if r.Workers <= 0 {
		return &validationError{"workers must be positive"}
	}
	return nil
}

type validationError struct {
	msg string
}

func (e *validationError) Error() string { return e.msg }

type WorkflowResponse struct {
	JobID     int64  `json:"job_id"`
	Tag       string `json:"tag"`
	State     string `json:"state"`
	CreatedAt string `json:"created_at"`
}

type JobStatusResponse struct {
	JobID       int64    `json:"job_id"`
	Kind        string   `json:"kind,omitempty"`
	State       string   `json:"state"`
	AttemptedAt string   `json:"attempted_at"`
	CompletedAt string   `json:"completed_at,omitempty"`
	Errors      []string `json:"errors,omitempty"`
}

type ChatRequest struct {
	Tag               string  `json:"tag"`
	Query             string  `json:"query"`
	ConversationID    string  `json:"conversation_id,omitempty"`
	TopK              int     `json:"top_k"`
	Temperature       float64 `json:"temperature"`
	MaxTokens         int     `json:"max_tokens"`
	LLMProvider       string  `json:"llm_provider"`
	LLMModel          string  `json:"llm_model"`
	EmbeddingProvider string  `json:"embedding_provider"`
	EmbeddingModel    string  `json:"embedding_model"`
}

func (r ChatRequest) Validate() error {
	if r.Tag == "" {
		return &validationError{"tag is required"}
	}
	if r.Query == "" {
		return &validationError{"query is required"}
	}
	if r.TopK <= 0 {
		return &validationError{"top_k must be positive"}
	}
	if r.Temperature <= 0 {
		return &validationError{"temperature must be positive"}
	}
	if r.MaxTokens <= 0 {
		return &validationError{"max_tokens must be positive"}
	}
	if r.LLMProvider == "" {
		return &validationError{"llm_provider is required"}
	}
	if r.LLMModel == "" {
		return &validationError{"llm_model is required"}
	}
	if r.EmbeddingProvider == "" {
		return &validationError{"embedding_provider is required"}
	}
	if r.EmbeddingModel == "" {
		return &validationError{"embedding_model is required"}
	}
	return nil
}

type ChatResponse struct {
	Answer          string      `json:"answer"`
	SourceDocuments []SourceDoc `json:"source_documents"`
	TokenUsage      TokenUsage  `json:"token_usage"`
	LatencyMs       int64       `json:"latency_ms"`
}

type SourceDoc struct {
	DocumentPath string  `json:"document_path"`
	Score        float32 `json:"score"`
}

type TokenUsage struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
}

type RunSummary struct {
	ID            string         `json:"id"`
	Tag           string         `json:"tag"`
	Strategy      map[string]any `json:"strategy"`
	Metrics       map[string]any `json:"metrics,omitempty"`
	QuestionCount int            `json:"question_count"`
	CreatedAt     string         `json:"created_at"`
}

type RunDetail struct {
	RunSummary
	Questions []map[string]any `json:"questions"`
	Total     int              `json:"total"`
}

type ArtifactEntry struct {
	Type      string `json:"type"`
	Tag       string `json:"tag"`
	FileCount *int   `json:"file_count"`
	CreatedAt string `json:"created_at,omitempty"`
}
