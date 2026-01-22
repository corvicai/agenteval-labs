-- Migration to add rating_code and score to evaluations table
ALTER TABLE evaluations ADD COLUMN IF NOT EXISTS rating_code INTEGER;
ALTER TABLE evaluations ADD COLUMN IF NOT EXISTS score INTEGER;

-- Backfill rating_code from rating
UPDATE evaluations SET rating_code = CASE
    WHEN rating = 'like' THEN 1
    WHEN rating = 'valid' THEN 2
    WHEN rating = 'dislike' THEN 3
    WHEN rating = 'wrong' THEN 4
    ELSE NULL
END WHERE rating_code IS NULL;

-- Backfill score from rating_code (0-100 scale)
UPDATE evaluations SET score = CASE
    WHEN rating_code = 1 THEN 100
    WHEN rating_code = 2 THEN 75
    WHEN rating_code = 3 THEN 25
    WHEN rating_code = 4 THEN 0
    ELSE NULL
END WHERE score IS NULL;

-- Optional: Add index for performance in stats aggregation
CREATE INDEX IF NOT EXISTS idx_evaluations_rating_code ON evaluations(rating_code);
