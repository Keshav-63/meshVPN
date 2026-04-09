-- Backfill missing user_id from requested_by for legacy rows.
-- Only backfill when requested_by already exists in users table.
UPDATE deployments d
SET user_id = d.requested_by
WHERE (d.user_id IS NULL OR d.user_id = '')
  AND d.requested_by IS NOT NULL
  AND d.requested_by <> ''
  AND EXISTS (
    SELECT 1
    FROM users u
    WHERE u.user_id = d.requested_by
  );
