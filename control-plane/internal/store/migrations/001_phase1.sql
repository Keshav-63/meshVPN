CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS organizations (
    organization_id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS organization_members (
    organization_id TEXT NOT NULL REFERENCES organizations(organization_id) ON DELETE CASCADE,
    user_id TEXT NOT NULL,
    role TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, user_id)
);

CREATE TABLE IF NOT EXISTS projects (
    project_id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(organization_id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    repo_owner TEXT,
    repo_name TEXT,
    repo_private BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, slug)
);

CREATE TABLE IF NOT EXISTS deployment_plans (
    plan_id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    cpu_cores DOUBLE PRECISION NOT NULL,
    memory_mb INTEGER NOT NULL,
    monthly_price_usd NUMERIC(10,2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO deployment_plans (plan_id, display_name, cpu_cores, memory_mb, monthly_price_usd)
VALUES
    ('nano', 'Nano', 0.25, 256, 0),
    ('small', 'Small', 0.50, 512, 0),
    ('medium', 'Medium', 1.00, 1024, 0)
ON CONFLICT (plan_id) DO NOTHING;

CREATE TABLE IF NOT EXISTS deployments (
    deployment_id TEXT PRIMARY KEY,
    project_id TEXT,
    requested_by TEXT,
    repo TEXT NOT NULL,
    subdomain TEXT NOT NULL,
    port INTEGER NOT NULL,
    cpu_cores DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_mb INTEGER NOT NULL DEFAULT 0,
    container TEXT,
    image TEXT,
    url TEXT,
    status TEXT NOT NULL,
    error TEXT,
    build_logs TEXT,
    env JSONB,
    build_args JSONB,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ
);

ALTER TABLE deployments ADD COLUMN IF NOT EXISTS project_id TEXT;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS requested_by TEXT;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS cpu_cores DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS memory_mb INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_deployments_started_at ON deployments(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_deployments_status ON deployments(status);
CREATE INDEX IF NOT EXISTS idx_deployments_project_id ON deployments(project_id);

CREATE TABLE IF NOT EXISTS deployment_jobs (
    job_id TEXT PRIMARY KEY,
    deployment_id TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    queued_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_deployment_jobs_status_queued_at ON deployment_jobs(status, queued_at);

CREATE TABLE IF NOT EXISTS deployment_events (
    event_id BIGSERIAL PRIMARY KEY,
    deployment_id TEXT NOT NULL,
    level TEXT NOT NULL,
    message TEXT NOT NULL,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_deployment_events_deployment_id ON deployment_events(deployment_id, created_at DESC);

CREATE TABLE IF NOT EXISTS deployment_logs_build (
    log_id BIGSERIAL PRIMARY KEY,
    deployment_id TEXT NOT NULL,
    stream TEXT NOT NULL,
    line TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_deployment_logs_build_deployment_id ON deployment_logs_build(deployment_id, created_at DESC);

CREATE TABLE IF NOT EXISTS usage_metrics_daily (
    metric_date DATE NOT NULL,
    organization_id TEXT NOT NULL,
    deployments_count INTEGER NOT NULL DEFAULT 0,
    successful_deployments_count INTEGER NOT NULL DEFAULT 0,
    failed_deployments_count INTEGER NOT NULL DEFAULT 0,
    build_minutes NUMERIC(10,2) NOT NULL DEFAULT 0,
    PRIMARY KEY (metric_date, organization_id)
);
