-- Backfill missing/empty package for legacy deployment records.
-- New deployments already default to package='small'.
UPDATE deployments
SET package = 'small'
WHERE package IS NULL OR package = '';
