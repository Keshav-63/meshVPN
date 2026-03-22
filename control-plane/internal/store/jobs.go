package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
)

var ErrNoQueuedJobs = errors.New("no queued jobs")

type JobRepository interface {
	Enqueue(ctx context.Context, job domain.DeploymentJob) error
	ClaimNext(ctx context.Context) (domain.DeploymentJob, error)
	MarkDone(ctx context.Context, jobID string) error
	MarkFailed(ctx context.Context, jobID string, errText string) error

	// Multi-worker methods
	AssignToWorker(ctx context.Context, jobID, workerID string) error
	ReleaseFromWorker(ctx context.Context, jobID string) error
	ClaimForWorker(ctx context.Context, workerID string) (domain.DeploymentJob, error)
	GetNextUnassignedJob(ctx context.Context) (domain.DeploymentJob, error)
}

type InMemoryJobRepository struct {
	mu   sync.Mutex
	jobs []domain.DeploymentJob
}

func NewInMemoryJobRepository() *InMemoryJobRepository {
	return &InMemoryJobRepository{jobs: make([]domain.DeploymentJob, 0)}
}

func (r *InMemoryJobRepository) Enqueue(_ context.Context, job domain.DeploymentJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	logs.Debugf("jobs-memory", "enqueue job job_id=%s deployment_id=%s", job.JobID, job.DeploymentID)
	r.jobs = append(r.jobs, job)
	return nil
}

func (r *InMemoryJobRepository) ClaimNext(_ context.Context) (domain.DeploymentJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.jobs) == 0 {
		return domain.DeploymentJob{}, ErrNoQueuedJobs
	}
	job := r.jobs[0]
	r.jobs = r.jobs[1:]
	logs.Debugf("jobs-memory", "claim job job_id=%s deployment_id=%s", job.JobID, job.DeploymentID)
	return job, nil
}

func (r *InMemoryJobRepository) MarkDone(_ context.Context, _ string) error {
	return nil
}

func (r *InMemoryJobRepository) MarkFailed(_ context.Context, _ string, _ string) error {
	return nil
}

// Multi-worker methods (not implemented for in-memory store)
func (r *InMemoryJobRepository) AssignToWorker(_ context.Context, _ string, _ string) error {
	return errors.New("AssignToWorker not implemented for in-memory store")
}

func (r *InMemoryJobRepository) ReleaseFromWorker(_ context.Context, _ string) error {
	return errors.New("ReleaseFromWorker not implemented for in-memory store")
}

func (r *InMemoryJobRepository) ClaimForWorker(_ context.Context, _ string) (domain.DeploymentJob, error) {
	return domain.DeploymentJob{}, errors.New("ClaimForWorker not implemented for in-memory store")
}

func (r *InMemoryJobRepository) GetNextUnassignedJob(_ context.Context) (domain.DeploymentJob, error) {
	return domain.DeploymentJob{}, errors.New("GetNextUnassignedJob not implemented for in-memory store")
}

type PostgresJobRepository struct {
	db *sql.DB
}

func NewPostgresJobRepository(db *sql.DB) *PostgresJobRepository {
	return &PostgresJobRepository{db: db}
}

func (r *PostgresJobRepository) Enqueue(ctx context.Context, job domain.DeploymentJob) error {
	const q = `
INSERT INTO deployment_jobs (job_id, deployment_id, payload, status, queued_at)
VALUES ($1, $2, $3::jsonb, 'queued', $4)
`
	payload, _ := json.Marshal(job)
	logs.Debugf("jobs-postgres", "enqueue job job_id=%s deployment_id=%s", job.JobID, job.DeploymentID)
	_, err := r.db.ExecContext(ctx, q, job.JobID, job.DeploymentID, string(payload), job.QueuedAt)
	return err
}

func (r *PostgresJobRepository) ClaimNext(ctx context.Context) (domain.DeploymentJob, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return domain.DeploymentJob{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	const q = `
WITH picked AS (
  SELECT job_id
  FROM deployment_jobs
  WHERE status = 'queued'
  ORDER BY queued_at ASC
  LIMIT 1
  FOR UPDATE SKIP LOCKED
)
UPDATE deployment_jobs j
SET status = 'running', started_at = NOW(), attempts = attempts + 1
FROM picked
WHERE j.job_id = picked.job_id
RETURNING j.job_id, j.payload
`

	var jobID string
	var payloadRaw []byte
	err = tx.QueryRowContext(ctx, q).Scan(&jobID, &payloadRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DeploymentJob{}, ErrNoQueuedJobs
	}
	if err != nil {
		return domain.DeploymentJob{}, err
	}

	var job domain.DeploymentJob
	if err := json.Unmarshal(payloadRaw, &job); err != nil {
		return domain.DeploymentJob{}, fmt.Errorf("decode job payload: %w", err)
	}
	job.JobID = jobID

	if err := tx.Commit(); err != nil {
		return domain.DeploymentJob{}, err
	}
	logs.Debugf("jobs-postgres", "claimed job job_id=%s deployment_id=%s", job.JobID, job.DeploymentID)

	return job, nil
}

func (r *PostgresJobRepository) MarkDone(ctx context.Context, jobID string) error {
	const q = `UPDATE deployment_jobs SET status='done', finished_at=NOW(), last_error=NULL WHERE job_id=$1`
	logs.Debugf("jobs-postgres", "mark done job_id=%s", jobID)
	_, err := r.db.ExecContext(ctx, q, jobID)
	return err
}

func (r *PostgresJobRepository) MarkFailed(ctx context.Context, jobID string, errText string) error {
	const q = `UPDATE deployment_jobs SET status='failed', finished_at=NOW(), last_error=$2 WHERE job_id=$1`
	logs.Debugf("jobs-postgres", "mark failed job_id=%s", jobID)
	_, err := r.db.ExecContext(ctx, q, jobID, errText)
	return err
}

// Multi-worker methods

// AssignToWorker assigns a queued job to a specific worker
func (r *PostgresJobRepository) AssignToWorker(ctx context.Context, jobID, workerID string) error {
	const q = `
UPDATE deployment_jobs
SET assigned_worker_id = $1, assigned_at = NOW()
WHERE job_id = $2 AND status = 'queued'
`
	logs.Debugf("jobs-postgres", "assign job to worker job_id=%s worker_id=%s", jobID, workerID)
	_, err := r.db.ExecContext(ctx, q, workerID, jobID)
	return err
}

// ReleaseFromWorker clears the worker assignment from a job
func (r *PostgresJobRepository) ReleaseFromWorker(ctx context.Context, jobID string) error {
	const q = `
UPDATE deployment_jobs
SET assigned_worker_id = NULL, assigned_at = NULL
WHERE job_id = $1
`
	logs.Debugf("jobs-postgres", "release job from worker job_id=%s", jobID)
	_, err := r.db.ExecContext(ctx, q, jobID)
	return err
}

// ClaimForWorker claims the next job assigned to a specific worker
func (r *PostgresJobRepository) ClaimForWorker(ctx context.Context, workerID string) (domain.DeploymentJob, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return domain.DeploymentJob{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	const q = `
WITH picked AS (
  SELECT job_id
  FROM deployment_jobs
  WHERE status = 'queued' AND assigned_worker_id = $1
  ORDER BY queued_at ASC
  LIMIT 1
  FOR UPDATE SKIP LOCKED
)
UPDATE deployment_jobs j
SET status = 'running', started_at = NOW(), attempts = attempts + 1
FROM picked
WHERE j.job_id = picked.job_id
RETURNING j.job_id, j.payload
`

	var jobID string
	var payloadRaw []byte
	err = tx.QueryRowContext(ctx, q, workerID).Scan(&jobID, &payloadRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DeploymentJob{}, ErrNoQueuedJobs
	}
	if err != nil {
		return domain.DeploymentJob{}, err
	}

	var job domain.DeploymentJob
	if err := json.Unmarshal(payloadRaw, &job); err != nil {
		return domain.DeploymentJob{}, fmt.Errorf("decode job payload: %w", err)
	}
	job.JobID = jobID

	if err := tx.Commit(); err != nil {
		return domain.DeploymentJob{}, err
	}
	logs.Debugf("jobs-postgres", "claimed job for worker job_id=%s worker_id=%s", job.JobID, workerID)

	return job, nil
}

// GetNextUnassignedJob retrieves the oldest queued job without a worker assignment
func (r *PostgresJobRepository) GetNextUnassignedJob(ctx context.Context) (domain.DeploymentJob, error) {
	const q = `
SELECT job_id, payload
FROM deployment_jobs
WHERE status = 'queued' AND (assigned_worker_id IS NULL OR assigned_worker_id = '')
ORDER BY queued_at ASC
LIMIT 1
`
	var jobID string
	var payloadRaw []byte

	err := r.db.QueryRowContext(ctx, q).Scan(&jobID, &payloadRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DeploymentJob{}, ErrNoQueuedJobs
	}
	if err != nil {
		return domain.DeploymentJob{}, err
	}

	var job domain.DeploymentJob
	if err := json.Unmarshal(payloadRaw, &job); err != nil {
		return domain.DeploymentJob{}, fmt.Errorf("decode job payload: %w", err)
	}
	job.JobID = jobID

	logs.Debugf("jobs-postgres", "found unassigned job job_id=%s", job.JobID)
	return job, nil
}
