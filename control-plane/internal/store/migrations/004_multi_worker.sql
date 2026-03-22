-- Multi-Worker Support Migration
-- This migration adds support for distributed worker agents

-- Workers table
CREATE TABLE IF NOT EXISTS workers (
    worker_id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    tailscale_ip TEXT NOT NULL,
    hostname TEXT,
    status TEXT NOT NULL DEFAULT 'idle',  -- idle, busy, offline
    capabilities JSONB,
    max_concurrent_jobs INT DEFAULT 1,
    current_jobs INT DEFAULT 0,
    last_heartbeat TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_workers_status ON workers(status);
CREATE INDEX IF NOT EXISTS idx_workers_last_heartbeat ON workers(last_heartbeat);

-- Add worker assignment columns to jobs table
ALTER TABLE deployment_jobs
ADD COLUMN IF NOT EXISTS assigned_worker_id TEXT REFERENCES workers(worker_id) ON DELETE SET NULL,
ADD COLUMN IF NOT EXISTS assigned_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_jobs_assigned_worker ON deployment_jobs(assigned_worker_id, status);
