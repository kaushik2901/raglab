CREATE TABLE IF NOT EXISTS eval_runs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id UUID NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    tag         TEXT NOT NULL,
    strategy    JSONB NOT NULL DEFAULT '{}',
    metrics     JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS eval_queries (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id        UUID NOT NULL REFERENCES eval_runs(id) ON DELETE CASCADE,
    question_id   TEXT NOT NULL,
    question      TEXT NOT NULL,
    expected      TEXT NOT NULL,
    expected_paths JSONB,
    retrieved     JSONB,
    answer        TEXT,
    hit           JSONB,
    rank_first    INT,
    prompt_tokens INT,
    completion_tokens INT,
    latency_ms    BIGINT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_eval_runs_tag ON eval_runs(tag);
CREATE INDEX IF NOT EXISTS idx_eval_runs_workflow ON eval_runs(workflow_id);
CREATE INDEX IF NOT EXISTS idx_eval_queries_run ON eval_queries(run_id);
