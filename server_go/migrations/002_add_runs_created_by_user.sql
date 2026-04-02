ALTER TABLE runs
ADD COLUMN IF NOT EXISTS created_by_user_id UUID REFERENCES users(id);

CREATE INDEX IF NOT EXISTS idx_runs_created_by_user_id ON runs(created_by_user_id);
