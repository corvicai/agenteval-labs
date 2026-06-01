-- Ensure each user or evaluator can have only one current evaluation per run result.
-- Keep the newest duplicate before adding the uniqueness guarantee.
DELETE FROM evaluations e
USING evaluations newer
WHERE e.run_result_id = newer.run_result_id
  AND e.rater_type = newer.rater_type
  AND COALESCE(e.rater_id, '00000000-0000-0000-0000-000000000000'::uuid) =
      COALESCE(newer.rater_id, '00000000-0000-0000-0000-000000000000'::uuid)
  AND (
    e.created_at < newer.created_at
    OR (e.created_at = newer.created_at AND e.id::text < newer.id::text)
  );

CREATE UNIQUE INDEX IF NOT EXISTS idx_evaluations_result_rater
ON evaluations (run_result_id, rater_type, COALESCE(rater_id, '00000000-0000-0000-0000-000000000000'::uuid));
