package api

type PreprocessRequest struct {
	RepoURL     string   `json:"repo_url"`
	Tag         string   `json:"tag,omitempty"`
	IncludeDirs []string `json:"include_dirs,omitempty"`
}

func (r PreprocessRequest) Validate() error {
	if r.RepoURL == "" {
		return &validationError{"repo_url is required"}
	}
	return nil
}

type IndexRequest struct {
	InputTag          string `json:"input_tag"`
	Tag               string `json:"tag,omitempty"`
	ParserStrategy    string `json:"parser_strategy,omitempty"`
	ChunkStrategy     string `json:"chunk_strategy,omitempty"`
	ChunkSize         int    `json:"chunk_size,omitempty"`
	ChunkOverlap      int    `json:"chunk_overlap,omitempty"`
	EmbeddingProvider string `json:"embedding_provider,omitempty"`
	EmbeddingModel    string `json:"embedding_model,omitempty"`
	BatchSize         int    `json:"batch_size,omitempty"`
	IndexConcurrency  int    `json:"index_concurrency,omitempty"`
	DocTimeout        string `json:"doc_timeout,omitempty"`
}

func (r IndexRequest) Validate() error {
	if r.InputTag == "" {
		return &validationError{"input_tag is required"}
	}
	return nil
}

type EvalRequest struct {
	IndexTag          string `json:"index_tag"`
	Tag               string `json:"tag,omitempty"`
	QueryStrategy     string `json:"query_strategy"`
	DatasetPath       string `json:"dataset_path"`
	TopK              int    `json:"top_k,omitempty"`
	Ks                []int  `json:"ks,omitempty"`
	LLMProvider       string `json:"llm_provider,omitempty"`
	LLMModel          string `json:"llm_model,omitempty"`
	EmbeddingProvider string `json:"embedding_provider,omitempty"`
	EmbeddingModel    string `json:"embedding_model,omitempty"`
	JudgeProvider     string `json:"judge_provider,omitempty"`
	JudgeModel        string `json:"judge_model,omitempty"`
	BatchSize         int    `json:"batch_size,omitempty"`
	Workers           int    `json:"workers,omitempty"`
}

func (r EvalRequest) Validate() error {
	if r.IndexTag == "" {
		return &validationError{"index_tag is required"}
	}
	if r.QueryStrategy == "" {
		return &validationError{"query_strategy is required"}
	}
	if r.DatasetPath == "" {
		return &validationError{"dataset_path is required"}
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
	Tag               string   `json:"tag"`
	Query             string   `json:"query"`
	ConversationID    string   `json:"conversation_id,omitempty"`
	TopK              int      `json:"top_k,omitempty"`
	Temperature       *float64 `json:"temperature,omitempty"`
	MaxTokens         int      `json:"max_tokens,omitempty"`
	LLMProvider       string   `json:"llm_provider,omitempty"`
	LLMModel          string   `json:"llm_model,omitempty"`
	EmbeddingProvider string   `json:"embedding_provider,omitempty"`
	EmbeddingModel    string   `json:"embedding_model,omitempty"`
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
