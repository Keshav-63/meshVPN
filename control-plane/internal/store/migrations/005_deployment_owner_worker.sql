-- Track which worker currently owns each deployment for failover and reconciliation.
ALTER TABLE deployments
ADD COLUMN IF NOT EXISTS owner_worker_id TEXT;

CREATE INDEX IF NOT EXISTS idx_deployments_owner_worker ON deployments(owner_worker_id, status);
