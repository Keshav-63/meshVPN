package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
)

var ErrNoAvailableWorkers = errors.New("no available workers")

// WorkerRepository defines operations for managing workers
type WorkerRepository interface {
	Register(ctx context.Context, worker domain.Worker) error
	Update(ctx context.Context, worker domain.Worker) error
	Get(ctx context.Context, workerID string) (domain.Worker, error)
	List(ctx context.Context) ([]domain.Worker, error)
	ListByStatus(ctx context.Context, status domain.WorkerStatus) ([]domain.Worker, error)
	UpdateHeartbeat(ctx context.Context, workerID string) error
	IncrementJobCount(ctx context.Context, workerID string) error
	DecrementJobCount(ctx context.Context, workerID string) error
	MarkOffline(ctx context.Context, workerID string) error
	GetAvailableWorker(ctx context.Context) (domain.Worker, error)
}

// PostgresWorkerRepository implements WorkerRepository using PostgreSQL
type PostgresWorkerRepository struct {
	db *sql.DB
}

// NewPostgresWorkerRepository creates a new Postgres-backed worker repository
func NewPostgresWorkerRepository(db *sql.DB) *PostgresWorkerRepository {
	return &PostgresWorkerRepository{db: db}
}

// Register inserts or updates a worker in the database
func (r *PostgresWorkerRepository) Register(ctx context.Context, worker domain.Worker) error {
	capabilitiesJSON, _ := json.Marshal(worker.Capabilities)

	const q = `
INSERT INTO workers (
	worker_id, name, tailscale_ip, hostname, status, capabilities,
	max_concurrent_jobs, current_jobs, last_heartbeat, created_at, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
ON CONFLICT (worker_id) DO UPDATE SET
	name = EXCLUDED.name,
	tailscale_ip = EXCLUDED.tailscale_ip,
	hostname = EXCLUDED.hostname,
	status = EXCLUDED.status,
	capabilities = EXCLUDED.capabilities,
	max_concurrent_jobs = EXCLUDED.max_concurrent_jobs,
	last_heartbeat = EXCLUDED.last_heartbeat,
	updated_at = NOW()
`
	logs.Debugf("workers-postgres", "register worker worker_id=%s name=%s", worker.WorkerID, worker.Name)
	_, err := r.db.ExecContext(ctx, q,
		worker.WorkerID, worker.Name, worker.TailscaleIP, worker.Hostname,
		worker.Status, string(capabilitiesJSON), worker.MaxConcurrentJobs,
		worker.CurrentJobs, time.Now(),
	)
	return err
}

// Update updates a worker's information
func (r *PostgresWorkerRepository) Update(ctx context.Context, worker domain.Worker) error {
	capabilitiesJSON, _ := json.Marshal(worker.Capabilities)

	const q = `
UPDATE workers SET
	name = $1,
	tailscale_ip = $2,
	hostname = $3,
	status = $4,
	capabilities = $5,
	max_concurrent_jobs = $6,
	current_jobs = $7,
	updated_at = NOW()
WHERE worker_id = $8
`
	logs.Debugf("workers-postgres", "update worker worker_id=%s status=%s current_jobs=%d",
		worker.WorkerID, worker.Status, worker.CurrentJobs)
	_, err := r.db.ExecContext(ctx, q,
		worker.Name, worker.TailscaleIP, worker.Hostname, worker.Status,
		string(capabilitiesJSON), worker.MaxConcurrentJobs, worker.CurrentJobs,
		worker.WorkerID,
	)
	return err
}

// Get retrieves a worker by ID
func (r *PostgresWorkerRepository) Get(ctx context.Context, workerID string) (domain.Worker, error) {
	const q = `
SELECT worker_id, name, tailscale_ip, hostname, status, capabilities,
       max_concurrent_jobs, current_jobs, last_heartbeat, created_at, updated_at
FROM workers
WHERE worker_id = $1
`
	var worker domain.Worker
	var capabilitiesRaw []byte
	var lastHeartbeat sql.NullTime

	err := r.db.QueryRowContext(ctx, q, workerID).Scan(
		&worker.WorkerID, &worker.Name, &worker.TailscaleIP, &worker.Hostname,
		&worker.Status, &capabilitiesRaw, &worker.MaxConcurrentJobs,
		&worker.CurrentJobs, &lastHeartbeat, &worker.CreatedAt, &worker.UpdatedAt,
	)
	if err != nil {
		return domain.Worker{}, err
	}

	if lastHeartbeat.Valid {
		worker.LastHeartbeat = &lastHeartbeat.Time
	}

	json.Unmarshal(capabilitiesRaw, &worker.Capabilities)

	return worker, nil
}

// List retrieves all workers
func (r *PostgresWorkerRepository) List(ctx context.Context) ([]domain.Worker, error) {
	const q = `
SELECT worker_id, name, tailscale_ip, hostname, status, capabilities,
       max_concurrent_jobs, current_jobs, last_heartbeat, created_at, updated_at
FROM workers
ORDER BY created_at DESC
`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workers []domain.Worker
	for rows.Next() {
		var worker domain.Worker
		var capabilitiesRaw []byte
		var lastHeartbeat sql.NullTime

		err := rows.Scan(
			&worker.WorkerID, &worker.Name, &worker.TailscaleIP, &worker.Hostname,
			&worker.Status, &capabilitiesRaw, &worker.MaxConcurrentJobs,
			&worker.CurrentJobs, &lastHeartbeat, &worker.CreatedAt, &worker.UpdatedAt,
		)
		if err != nil {
			continue
		}

		if lastHeartbeat.Valid {
			worker.LastHeartbeat = &lastHeartbeat.Time
		}

		json.Unmarshal(capabilitiesRaw, &worker.Capabilities)
		workers = append(workers, worker)
	}

	return workers, nil
}

// ListByStatus retrieves workers filtered by status
func (r *PostgresWorkerRepository) ListByStatus(ctx context.Context, status domain.WorkerStatus) ([]domain.Worker, error) {
	const q = `
SELECT worker_id, name, tailscale_ip, hostname, status, capabilities,
       max_concurrent_jobs, current_jobs, last_heartbeat, created_at, updated_at
FROM workers
WHERE status = $1
ORDER BY current_jobs ASC, created_at ASC
`
	rows, err := r.db.QueryContext(ctx, q, string(status))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workers []domain.Worker
	for rows.Next() {
		var worker domain.Worker
		var capabilitiesRaw []byte
		var lastHeartbeat sql.NullTime

		err := rows.Scan(
			&worker.WorkerID, &worker.Name, &worker.TailscaleIP, &worker.Hostname,
			&worker.Status, &capabilitiesRaw, &worker.MaxConcurrentJobs,
			&worker.CurrentJobs, &lastHeartbeat, &worker.CreatedAt, &worker.UpdatedAt,
		)
		if err != nil {
			continue
		}

		if lastHeartbeat.Valid {
			worker.LastHeartbeat = &lastHeartbeat.Time
		}

		json.Unmarshal(capabilitiesRaw, &worker.Capabilities)
		workers = append(workers, worker)
	}

	return workers, nil
}

// UpdateHeartbeat updates the last heartbeat timestamp for a worker
func (r *PostgresWorkerRepository) UpdateHeartbeat(ctx context.Context, workerID string) error {
	const q = `UPDATE workers SET last_heartbeat = NOW(), updated_at = NOW() WHERE worker_id = $1`
	logs.Debugf("workers-postgres", "heartbeat worker_id=%s", workerID)
	_, err := r.db.ExecContext(ctx, q, workerID)
	return err
}

// IncrementJobCount increments the current job count for a worker
func (r *PostgresWorkerRepository) IncrementJobCount(ctx context.Context, workerID string) error {
	const q = `
UPDATE workers SET
	current_jobs = current_jobs + 1,
	status = CASE
		WHEN current_jobs + 1 >= max_concurrent_jobs THEN 'busy'
		ELSE status
	END,
	updated_at = NOW()
WHERE worker_id = $1
`
	logs.Debugf("workers-postgres", "increment job count worker_id=%s", workerID)
	_, err := r.db.ExecContext(ctx, q, workerID)
	return err
}

// DecrementJobCount decrements the current job count for a worker
func (r *PostgresWorkerRepository) DecrementJobCount(ctx context.Context, workerID string) error {
	const q = `
UPDATE workers SET
	current_jobs = GREATEST(current_jobs - 1, 0),
	status = CASE
		WHEN current_jobs - 1 < max_concurrent_jobs AND status = 'busy' THEN 'idle'
		ELSE status
	END,
	updated_at = NOW()
WHERE worker_id = $1
`
	logs.Debugf("workers-postgres", "decrement job count worker_id=%s", workerID)
	_, err := r.db.ExecContext(ctx, q, workerID)
	return err
}

// MarkOffline marks a worker as offline
func (r *PostgresWorkerRepository) MarkOffline(ctx context.Context, workerID string) error {
	const q = `UPDATE workers SET status = 'offline', updated_at = NOW() WHERE worker_id = $1`
	logs.Infof("workers-postgres", "marking worker offline worker_id=%s", workerID)
	_, err := r.db.ExecContext(ctx, q, workerID)
	return err
}

// GetAvailableWorker finds an idle worker with available job slots
func (r *PostgresWorkerRepository) GetAvailableWorker(ctx context.Context) (domain.Worker, error) {
	const q = `
SELECT worker_id, name, tailscale_ip, hostname, status, capabilities,
       max_concurrent_jobs, current_jobs, last_heartbeat, created_at, updated_at
FROM workers
WHERE status = 'idle'
  AND current_jobs < max_concurrent_jobs
ORDER BY current_jobs ASC, created_at ASC
LIMIT 1
`
	var worker domain.Worker
	var capabilitiesRaw []byte
	var lastHeartbeat sql.NullTime

	err := r.db.QueryRowContext(ctx, q).Scan(
		&worker.WorkerID, &worker.Name, &worker.TailscaleIP, &worker.Hostname,
		&worker.Status, &capabilitiesRaw, &worker.MaxConcurrentJobs,
		&worker.CurrentJobs, &lastHeartbeat, &worker.CreatedAt, &worker.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Worker{}, ErrNoAvailableWorkers
	}
	if err != nil {
		return domain.Worker{}, err
	}

	if lastHeartbeat.Valid {
		worker.LastHeartbeat = &lastHeartbeat.Time
	}

	json.Unmarshal(capabilitiesRaw, &worker.Capabilities)

	logs.Debugf("workers-postgres", "found available worker worker_id=%s current_jobs=%d/%d",
		worker.WorkerID, worker.CurrentJobs, worker.MaxConcurrentJobs)
	return worker, nil
}
