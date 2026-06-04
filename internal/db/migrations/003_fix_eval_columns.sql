ALTER TABLE eval_queries RENAME COLUMN expected TO generated_answer;
ALTER TABLE eval_queries ADD COLUMN relevance JSONB;
