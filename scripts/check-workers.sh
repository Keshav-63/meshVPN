#!/bin/bash
# Check Worker Registration Status
# Run this on control-plane to verify workers are registered

set -e

# Load environment
if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
fi

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo "=========================================="
echo "MeshVPN Worker Status Check"
echo "=========================================="
echo ""

# Check if database URL is set
if [ -z "$DATABASE_URL" ]; then
    echo -e "${RED}ERROR: DATABASE_URL not set in .env${NC}"
    exit 1
fi

echo "Checking workers in database..."
echo ""

# Query workers
psql "$DATABASE_URL" -c "
SELECT
    worker_id,
    name,
    tailscale_ip,
    status,
    current_jobs || '/' || max_concurrent_jobs as jobs,
    CASE
        WHEN last_heartbeat IS NULL THEN 'Never'
        WHEN NOW() - last_heartbeat < INTERVAL '1 minute' THEN 'Just now'
        ELSE EXTRACT(EPOCH FROM (NOW() - last_heartbeat))::INT || 's ago'
    END as last_seen
FROM workers
ORDER BY created_at ASC;
"

echo ""
echo "Job Distribution:"
psql "$DATABASE_URL" -c "
SELECT
    assigned_worker_id,
    status,
    COUNT(*) as count
FROM deployment_jobs
WHERE assigned_worker_id IS NOT NULL
GROUP BY assigned_worker_id, status
ORDER BY assigned_worker_id, status;
"

echo ""
echo "Current System Status:"
echo ""

# Count workers by status
TOTAL=$(psql "$DATABASE_URL" -t -c "SELECT COUNT(*) FROM workers;")
IDLE=$(psql "$DATABASE_URL" -t -c "SELECT COUNT(*) FROM workers WHERE status='idle';")
BUSY=$(psql "$DATABASE_URL" -t -c "SELECT COUNT(*) FROM workers WHERE status='busy';")
OFFLINE=$(psql "$DATABASE_URL" -t -c "SELECT COUNT(*) FROM workers WHERE status='offline';")

echo -e "Total Workers:   ${GREEN}$TOTAL${NC}"
echo -e "Idle:            ${GREEN}$IDLE${NC}"
echo -e "Busy:            ${YELLOW}$BUSY${NC}"
echo -e "Offline:         ${RED}$OFFLINE${NC}"

echo ""

# Calculate total capacity
TOTAL_CAPACITY=$(psql "$DATABASE_URL" -t -c "SELECT SUM(max_concurrent_jobs) FROM workers WHERE status IN ('idle', 'busy');")
USED_CAPACITY=$(psql "$DATABASE_URL" -t -c "SELECT SUM(current_jobs) FROM workers WHERE status IN ('idle', 'busy');")

echo "Capacity: $USED_CAPACITY / $TOTAL_CAPACITY jobs"

# Queue depth
QUEUED=$(psql "$DATABASE_URL" -t -c "SELECT COUNT(*) FROM deployment_jobs WHERE status='queued';")
RUNNING=$(psql "$DATABASE_URL" -t -c "SELECT COUNT(*) FROM deployment_jobs WHERE status='running';")

echo "Queue: $QUEUED queued, $RUNNING running"

echo ""
echo "=========================================="
