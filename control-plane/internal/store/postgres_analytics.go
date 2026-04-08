package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
)

type PostgresAnalyticsRepository struct {
	db *sql.DB
}

func NewPostgresAnalyticsRepository(db *sql.DB) *PostgresAnalyticsRepository {
	return &PostgresAnalyticsRepository{db: db}
}

// GetMetrics retrieves the current aggregated metrics for a deployment
func (r *PostgresAnalyticsRepository) GetMetrics(deploymentID string) (domain.DeploymentMetrics, error) {
	const query = `
SELECT deployment_id, request_count_total, request_count_1h, request_count_24h,
       requests_per_second, bandwidth_sent_bytes, bandwidth_received_bytes,
       latency_p50_ms, latency_p90_ms, latency_p99_ms,
       current_pods, desired_pods, cpu_usage_percent, memory_usage_mb, last_updated
FROM deployment_metrics
WHERE deployment_id = $1
`

	var metrics domain.DeploymentMetrics
	var latencyP50, latencyP90, latencyP99 sql.NullFloat64
	var cpuUsage, memoryUsage sql.NullFloat64

	err := r.db.QueryRow(query, deploymentID).Scan(
		&metrics.DeploymentID,
		&metrics.RequestCountTotal,
		&metrics.RequestCount1h,
		&metrics.RequestCount24h,
		&metrics.RequestsPerSecond,
		&metrics.BandwidthSentBytes,
		&metrics.BandwidthRecvBytes,
		&latencyP50,
		&latencyP90,
		&latencyP99,
		&metrics.CurrentPods,
		&metrics.DesiredPods,
		&cpuUsage,
		&memoryUsage,
		&metrics.LastUpdated,
	)

	if err == sql.ErrNoRows {
		// No metrics yet, return zero values
		return domain.DeploymentMetrics{
			DeploymentID: deploymentID,
			LastUpdated:  time.Now(),
		}, nil
	}

	if err != nil {
		return domain.DeploymentMetrics{}, fmt.Errorf("query metrics: %w", err)
	}

	if latencyP50.Valid {
		metrics.LatencyP50Ms = latencyP50.Float64
	}
	if latencyP90.Valid {
		metrics.LatencyP90Ms = latencyP90.Float64
	}
	if latencyP99.Valid {
		metrics.LatencyP99Ms = latencyP99.Float64
	}
	if cpuUsage.Valid {
		metrics.CPUUsagePercent = cpuUsage.Float64
	}
	if memoryUsage.Valid {
		metrics.MemoryUsageMB = memoryUsage.Float64
	}

	return metrics, nil
}

// UpdateMetrics updates or inserts aggregated metrics for a deployment
func (r *PostgresAnalyticsRepository) UpdateMetrics(metrics domain.DeploymentMetrics) error {
	const stmt = `
INSERT INTO deployment_metrics (
	deployment_id, request_count_total, request_count_1h, request_count_24h,
	requests_per_second, bandwidth_sent_bytes, bandwidth_received_bytes,
	latency_p50_ms, latency_p90_ms, latency_p99_ms,
	current_pods, desired_pods, cpu_usage_percent, memory_usage_mb, last_updated
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
ON CONFLICT (deployment_id)
DO UPDATE SET
	request_count_total = EXCLUDED.request_count_total,
	request_count_1h = EXCLUDED.request_count_1h,
	request_count_24h = EXCLUDED.request_count_24h,
	requests_per_second = EXCLUDED.requests_per_second,
	bandwidth_sent_bytes = EXCLUDED.bandwidth_sent_bytes,
	bandwidth_received_bytes = EXCLUDED.bandwidth_received_bytes,
	latency_p50_ms = EXCLUDED.latency_p50_ms,
	latency_p90_ms = EXCLUDED.latency_p90_ms,
	latency_p99_ms = EXCLUDED.latency_p99_ms,
	current_pods = EXCLUDED.current_pods,
	desired_pods = EXCLUDED.desired_pods,
	cpu_usage_percent = EXCLUDED.cpu_usage_percent,
	memory_usage_mb = EXCLUDED.memory_usage_mb,
	last_updated = EXCLUDED.last_updated
`

	_, err := r.db.Exec(stmt,
		metrics.DeploymentID,
		metrics.RequestCountTotal,
		metrics.RequestCount1h,
		metrics.RequestCount24h,
		metrics.RequestsPerSecond,
		metrics.BandwidthSentBytes,
		metrics.BandwidthRecvBytes,
		nullFloat(metrics.LatencyP50Ms),
		nullFloat(metrics.LatencyP90Ms),
		nullFloat(metrics.LatencyP99Ms),
		metrics.CurrentPods,
		metrics.DesiredPods,
		nullFloat(metrics.CPUUsagePercent),
		nullFloat(metrics.MemoryUsageMB),
		time.Now().UTC(),
	)

	if err != nil {
		return fmt.Errorf("update metrics: %w", err)
	}

	return nil
}

// RecordRequest logs an individual request for percentile calculation
func (r *PostgresAnalyticsRepository) RecordRequest(req domain.DeploymentRequest) error {
	deploymentID, err := r.resolveDeploymentID(req.DeploymentID)
	if err != nil {
		return fmt.Errorf("resolve deployment id: %w", err)
	}
	if deploymentID == "" {
		// Unknown deployment identifier. Keep telemetry fire-and-forget and skip.
		return nil
	}

	const stmt = `
INSERT INTO deployment_requests (
	deployment_id, timestamp, status_code, latency_ms, bytes_sent, bytes_received, path
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
`

	_, err = r.db.Exec(stmt,
		deploymentID,
		req.Timestamp,
		req.StatusCode,
		req.LatencyMs,
		req.BytesSent,
		req.BytesReceived,
		req.Path,
	)

	if err != nil {
		return fmt.Errorf("record request: %w", err)
	}

	return nil
}

// RecordRequestBatch inserts many resolved deployment requests in a single SQL statement.
// Requests with empty deployment IDs are ignored.
func (r *PostgresAnalyticsRepository) RecordRequestBatch(requests []domain.DeploymentRequest) error {
	if len(requests) == 0 {
		return nil
	}

	const columnsPerRow = 7

	var queryBuilder strings.Builder
	queryBuilder.WriteString(`
INSERT INTO deployment_requests (
	deployment_id, timestamp, status_code, latency_ms, bytes_sent, bytes_received, path
)
VALUES
`)

	args := make([]interface{}, 0, len(requests)*columnsPerRow)
	paramIdx := 1
	rowsAdded := 0

	for _, req := range requests {
		deploymentID := strings.TrimSpace(req.DeploymentID)
		if deploymentID == "" {
			continue
		}

		if rowsAdded > 0 {
			queryBuilder.WriteString(",\n")
		}

		queryBuilder.WriteString(fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			paramIdx,
			paramIdx+1,
			paramIdx+2,
			paramIdx+3,
			paramIdx+4,
			paramIdx+5,
			paramIdx+6,
		))

		args = append(args,
			deploymentID,
			req.Timestamp,
			req.StatusCode,
			req.LatencyMs,
			req.BytesSent,
			req.BytesReceived,
			req.Path,
		)

		paramIdx += columnsPerRow
		rowsAdded++
	}

	if rowsAdded == 0 {
		return nil
	}

	if _, err := r.db.Exec(queryBuilder.String(), args...); err != nil {
		return fmt.Errorf("record request batch: %w", err)
	}

	return nil
}

func (r *PostgresAnalyticsRepository) resolveDeploymentID(identifier string) (string, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return "", nil
	}

	var deploymentID string

	// First, prefer exact deployment_id matches (supports short IDs like dd33c835).
	const byIDQuery = `SELECT deployment_id FROM deployments WHERE deployment_id = $1 LIMIT 1`
	err := r.db.QueryRow(byIDQuery, identifier).Scan(&deploymentID)
	if err == nil {
		return deploymentID, nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("lookup by deployment_id: %w", err)
	}

	// Fallback: allow telemetry producers that still send subdomain values.
	const bySubdomainQuery = `SELECT deployment_id FROM deployments WHERE subdomain = $1 LIMIT 1`
	err = r.db.QueryRow(bySubdomainQuery, identifier).Scan(&deploymentID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("lookup by subdomain: %w", err)
	}

	return deploymentID, nil
}

// CalculatePercentiles calculates p50, p90, p99 latencies from recent requests
func (r *PostgresAnalyticsRepository) CalculatePercentiles(deploymentID string, duration time.Duration) (p50, p90, p99 float64, err error) {
	since := time.Now().Add(-duration)

	const query = `
SELECT
	PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY latency_ms) AS p50,
	PERCENTILE_CONT(0.90) WITHIN GROUP (ORDER BY latency_ms) AS p90,
	PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY latency_ms) AS p99
FROM deployment_requests
WHERE deployment_id = $1 AND timestamp >= $2
`

	var p50Null, p90Null, p99Null sql.NullFloat64

	err = r.db.QueryRow(query, deploymentID, since).Scan(&p50Null, &p90Null, &p99Null)
	if err == sql.ErrNoRows {
		return 0, 0, 0, nil // No data, return zeros
	}
	if err != nil {
		return 0, 0, 0, fmt.Errorf("calculate percentiles: %w", err)
	}

	if p50Null.Valid {
		p50 = p50Null.Float64
	}
	if p90Null.Valid {
		p90 = p90Null.Float64
	}
	if p99Null.Valid {
		p99 = p99Null.Float64
	}

	return p50, p90, p99, nil
}

// GetRequestCounts retrieves request counts for different time windows
func (r *PostgresAnalyticsRepository) GetRequestCounts(deploymentID string) (total, last1h, last24h int64, err error) {
	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)
	oneDayAgo := now.Add(-24 * time.Hour)

	// Total requests
	const totalQuery = `SELECT COUNT(*) FROM deployment_requests WHERE deployment_id = $1`
	err = r.db.QueryRow(totalQuery, deploymentID).Scan(&total)
	if err != nil && err != sql.ErrNoRows {
		return 0, 0, 0, fmt.Errorf("query total requests: %w", err)
	}

	// Last 1 hour
	const hourQuery = `SELECT COUNT(*) FROM deployment_requests WHERE deployment_id = $1 AND timestamp >= $2`
	err = r.db.QueryRow(hourQuery, deploymentID, oneHourAgo).Scan(&last1h)
	if err != nil && err != sql.ErrNoRows {
		return 0, 0, 0, fmt.Errorf("query 1h requests: %w", err)
	}

	// Last 24 hours
	const dayQuery = `SELECT COUNT(*) FROM deployment_requests WHERE deployment_id = $1 AND timestamp >= $2`
	err = r.db.QueryRow(dayQuery, deploymentID, oneDayAgo).Scan(&last24h)
	if err != nil && err != sql.ErrNoRows {
		return 0, 0, 0, fmt.Errorf("query 24h requests: %w", err)
	}

	return total, last1h, last24h, nil
}

// GetBandwidthStats retrieves total bandwidth sent and received
func (r *PostgresAnalyticsRepository) GetBandwidthStats(deploymentID string) (sent, received int64, err error) {
	const query = `
SELECT
	COALESCE(SUM(bytes_sent), 0) AS total_sent,
	COALESCE(SUM(bytes_received), 0) AS total_received
FROM deployment_requests
WHERE deployment_id = $1
`

	err = r.db.QueryRow(query, deploymentID).Scan(&sent, &received)
	if err == sql.ErrNoRows {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("query bandwidth: %w", err)
	}

	return sent, received, nil
}

// CleanupOldRequests deletes request logs older than the specified time
func (r *PostgresAnalyticsRepository) CleanupOldRequests(olderThan time.Time) error {
	const stmt = `DELETE FROM deployment_requests WHERE timestamp < $1`

	result, err := r.db.Exec(stmt, olderThan)
	if err != nil {
		return fmt.Errorf("cleanup old requests: %w", err)
	}

	rowsDeleted, _ := result.RowsAffected()
	if rowsDeleted > 0 {
		logs.Infof("analytics", "cleaned up %d old request records", rowsDeleted)
	}

	return nil
}

// GetAllActiveDeploymentIDs returns all deployment IDs that have running status
func (r *PostgresAnalyticsRepository) GetAllActiveDeploymentIDs() ([]string, error) {
	const query = `
SELECT DISTINCT deployment_id
FROM deployments
WHERE status = 'running'
ORDER BY deployment_id
`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query active deployments: %w", err)
	}
	defer rows.Close()

	var deploymentIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		deploymentIDs = append(deploymentIDs, id)
	}

	return deploymentIDs, nil
}

// GetDeploymentSummaries retrieves summary metrics for multiple deployments efficiently
func (r *PostgresAnalyticsRepository) GetDeploymentSummaries(deploymentIDs []string) (map[string]domain.DeploymentMetrics, error) {
	if len(deploymentIDs) == 0 {
		return make(map[string]domain.DeploymentMetrics), nil
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(deploymentIDs))
	args := make([]interface{}, len(deploymentIDs))
	for i, id := range deploymentIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf(`
SELECT deployment_id, request_count_total, request_count_1h, request_count_24h,
       requests_per_second, bandwidth_sent_bytes, bandwidth_received_bytes,
       latency_p50_ms, latency_p90_ms, latency_p99_ms,
       current_pods, desired_pods, cpu_usage_percent, memory_usage_mb, last_updated
FROM deployment_metrics
WHERE deployment_id IN (%s)
`, strings.Join(placeholders, ", "))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query deployment summaries: %w", err)
	}
	defer rows.Close()

	result := make(map[string]domain.DeploymentMetrics)
	for rows.Next() {
		var metrics domain.DeploymentMetrics
		var latencyP50, latencyP90, latencyP99 sql.NullFloat64
		var cpuUsage, memoryUsage sql.NullFloat64

		err := rows.Scan(
			&metrics.DeploymentID,
			&metrics.RequestCountTotal,
			&metrics.RequestCount1h,
			&metrics.RequestCount24h,
			&metrics.RequestsPerSecond,
			&metrics.BandwidthSentBytes,
			&metrics.BandwidthRecvBytes,
			&latencyP50,
			&latencyP90,
			&latencyP99,
			&metrics.CurrentPods,
			&metrics.DesiredPods,
			&cpuUsage,
			&memoryUsage,
			&metrics.LastUpdated,
		)

		if err != nil {
			logs.Errorf("analytics", "failed to scan metrics row: %v", err)
			continue
		}

		if latencyP50.Valid {
			metrics.LatencyP50Ms = latencyP50.Float64
		}
		if latencyP90.Valid {
			metrics.LatencyP90Ms = latencyP90.Float64
		}
		if latencyP99.Valid {
			metrics.LatencyP99Ms = latencyP99.Float64
		}
		if cpuUsage.Valid {
			metrics.CPUUsagePercent = cpuUsage.Float64
		}
		if memoryUsage.Valid {
			metrics.MemoryUsageMB = memoryUsage.Float64
		}

		result[metrics.DeploymentID] = metrics
	}

	return result, nil
}

// Helper function to convert float64 to sql.NullFloat64
func nullFloat(f float64) sql.NullFloat64 {
	if f == 0 {
		return sql.NullFloat64{Valid: false}
	}
	return sql.NullFloat64{Float64: f, Valid: true}
}
