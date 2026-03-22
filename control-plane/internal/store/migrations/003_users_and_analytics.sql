-- Users/Organizations table for subscription tracking
CREATE TABLE IF NOT EXISTS users (
    user_id TEXT PRIMARY KEY,
    email TEXT,
    provider TEXT,
    is_subscriber BOOLEAN DEFAULT false,
    subscription_tier TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_subscriber ON users(is_subscriber);

-- Add user tracking and package fields to deployments
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS package TEXT DEFAULT 'small';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS user_id TEXT REFERENCES users(user_id);

CREATE INDEX IF NOT EXISTS idx_deployments_user_id ON deployments(user_id);
CREATE INDEX IF NOT EXISTS idx_deployments_package ON deployments(package);

-- Deployment metrics aggregated table (updated every minute by collector)
CREATE TABLE IF NOT EXISTS deployment_metrics (
    deployment_id TEXT PRIMARY KEY REFERENCES deployments(deployment_id) ON DELETE CASCADE,
    request_count_total BIGINT DEFAULT 0,
    request_count_1h BIGINT DEFAULT 0,
    request_count_24h BIGINT DEFAULT 0,
    requests_per_second FLOAT DEFAULT 0,
    bandwidth_sent_bytes BIGINT DEFAULT 0,
    bandwidth_received_bytes BIGINT DEFAULT 0,
    latency_p50_ms FLOAT,
    latency_p90_ms FLOAT,
    latency_p99_ms FLOAT,
    current_pods INT DEFAULT 0,
    desired_pods INT DEFAULT 0,
    cpu_usage_percent FLOAT,
    memory_usage_mb FLOAT,
    last_updated TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_deployment_metrics_updated ON deployment_metrics(last_updated DESC);

-- Request logs for percentile calculation
CREATE TABLE IF NOT EXISTS deployment_requests (
    id BIGSERIAL PRIMARY KEY,
    deployment_id TEXT NOT NULL REFERENCES deployments(deployment_id) ON DELETE CASCADE,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status_code INT,
    latency_ms FLOAT,
    bytes_sent BIGINT,
    bytes_received BIGINT,
    path TEXT
);

CREATE INDEX IF NOT EXISTS idx_requests_deployment_timestamp ON deployment_requests(deployment_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_requests_timestamp ON deployment_requests(timestamp DESC);

-- Note: Cleanup of old request logs (older than 7 days) should be handled by a background job
