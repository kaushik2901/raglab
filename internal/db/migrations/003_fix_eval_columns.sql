ALTER TABLE eval_queries RENAME COLUMN expected TO generated_answer;
ALTER TABLE eval_queries ADD COLUMN relevance JSONB;
ALTER TABLE eval_queries ADD COLUMN answer_score DOUBLE PRECISION NOT NULL DEFAULT 0;
