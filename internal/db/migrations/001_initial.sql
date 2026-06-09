CREATE TABLE IF NOT EXISTS eval_runs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tag         TEXT NOT NULL,
    strategy    JSONB NOT NULL DEFAULT '{}',
    metrics     JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Clean up legacy column from earlier schema
ALTER TABLE eval_runs DROP COLUMN IF EXISTS workflow_id;

CREATE INDEX IF NOT EXISTS idx_eval_runs_tag ON eval_runs(tag);

CREATE TABLE IF NOT EXISTS eval_queries (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id        UUID NOT NULL REFERENCES eval_runs(id) ON DELETE CASCADE,
    question_id   TEXT NOT NULL,
    question      TEXT NOT NULL,
    generated_answer TEXT,
    expected_answer  TEXT NOT NULL DEFAULT '',
    expected_paths JSONB,
    retrieved     JSONB,
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
