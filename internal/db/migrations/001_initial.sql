CREATE TABLE IF NOT EXISTS workflows (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type        TEXT NOT NULL,
    tag         TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    input_params JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_workflows_tag ON workflows(tag);
CREATE INDEX IF NOT EXISTS idx_workflows_type ON workflows(type);
CREATE INDEX IF NOT EXISTS idx_workflows_status ON workflows(status);

CREATE TABLE IF NOT EXISTS workflow_steps (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id UUID NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    step_name   TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    attempts    INT NOT NULL DEFAULT 0,
    error       TEXT,
    output      JSONB,
    started_at  TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_workflow_steps_wf ON workflow_steps(workflow_id);

CREATE TABLE IF NOT EXISTS eval_runs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id UUID NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    tag         TEXT NOT NULL,
    strategy    JSONB NOT NULL DEFAULT '{}',
    metrics     JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_eval_runs_tag ON eval_runs(tag);
CREATE INDEX IF NOT EXISTS idx_eval_runs_workflow ON eval_runs(workflow_id);

CREATE TABLE IF NOT EXISTS eval_queries (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id        UUID NOT NULL REFERENCES eval_runs(id) ON DELETE CASCADE,
    question_id   TEXT NOT NULL,
    question      TEXT NOT NULL,
    generated_answer TEXT,
    expected_answer  TEXT NOT NULL DEFAULT '',
    expected_paths JSONB,
    retrieved     JSONB,
    answer        TEXT,
    hit           JSONB,
    rank_first    INT,
    relevance     JSONB,
    answer_score  DOUBLE PRECISION NOT NULL DEFAULT 0,
    category      TEXT NOT NULL DEFAULT '',
    difficulty    TEXT NOT NULL DEFAULT '',
    ndcg_graded   DOUBLE PRECISION NOT NULL DEFAULT 0,
    prompt_tokens INT,
    completion_tokens INT,
    latency_ms    BIGINT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_eval_queries_run ON eval_queries(run_id);
