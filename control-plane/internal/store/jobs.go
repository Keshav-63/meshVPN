package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

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
	jobs []inMemoryJobState
}

type inMemoryJobState struct {
	job        domain.DeploymentJob
	status     string
	lastError  string
	finishedAt *time.Time
}

func NewInMemoryJobRepository() *InMemoryJobRepository {
	return &InMemoryJobRepository{jobs: make([]inMemoryJobState, 0)}
}

func (r *InMemoryJobRepository) Enqueue(_ context.Context, job domain.DeploymentJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	logs.Debugf("jobs-memory", "enqueue job job_id=%s deployment_id=%s requested_by=%s repo=%s queue_size_before=%d",
		job.JobID, job.DeploymentID, job.RequestedBy, job.Repo, len(r.jobs))
	r.jobs = append(r.jobs, inMemoryJobState{job: job, status: "queued"})
	logs.Debugf("jobs-memory", "enqueue complete job_id=%s queue_size_after=%d", job.JobID, len(r.jobs))
	return nil
}

func (r *InMemoryJobRepository) ClaimNext(_ context.Context) (domain.DeploymentJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.jobs {
		if r.jobs[i].status != "queued" {
			continue
		}
		r.jobs[i].status = "running"
		now := time.Now()
		r.jobs[i].job.AssignedAt = &now
		logs.Debugf("jobs-memory", "claim job job_id=%s deployment_id=%s requested_by=%s repo=%s queue_size=%d",
			r.jobs[i].job.JobID, r.jobs[i].job.DeploymentID, r.jobs[i].job.RequestedBy, r.jobs[i].job.Repo, len(r.jobs))
		return r.jobs[i].job, nil
	}
	if len(r.jobs) == 0 {
		return domain.DeploymentJob{}, ErrNoQueuedJobs
	}
	return domain.DeploymentJob{}, ErrNoQueuedJobs
}

func (r *InMemoryJobRepository) MarkDone(ctx context.Context, jobID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, job := range r.jobs {
		if job.job.JobID == jobID {
			now := time.Now()
			r.jobs[i].status = "done"
			r.jobs[i].finishedAt = &now
			logs.Debugf("jobs-memory", "mark done job_id=%s deployment_id=%s queue_size=%d",
				jobID, job.job.DeploymentID, len(r.jobs))
			return nil
		}
	}
	return fmt.Errorf("job not found: %s", jobID)
}

func (r *InMemoryJobRepository) MarkFailed(ctx context.Context, jobID string, errText string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, job := range r.jobs {
		if job.job.JobID == jobID {
			now := time.Now()
			r.jobs[i].status = "failed"
			r.jobs[i].lastError = errText
			r.jobs[i].finishedAt = &now
			logs.Warnf("jobs-memory", "mark failed job_id=%s deployment_id=%s error=%s queue_size=%d",
				jobID, job.job.DeploymentID, errText, len(r.jobs))
			return nil
		}
	}
	return fmt.Errorf("job not found: %s", jobID)
}

// Multi-worker methods (implemented for in-memory store)
func (r *InMemoryJobRepository) AssignToWorker(_ context.Context, jobID, workerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, job := range r.jobs {
		if job.job.JobID == jobID {
			if r.jobs[i].status != "queued" {
				return fmt.Errorf("job not assignable (status=%s): %s", r.jobs[i].status, jobID)
			}
			r.jobs[i].job.AssignedWorkerID = workerID
			now := time.Now()
			r.jobs[i].job.AssignedAt = &now
			logs.Debugf("jobs-memory", "assign job to worker job_id=%s deployment_id=%s worker_id=%s status=%s",
				jobID, job.job.DeploymentID, workerID, job.status)
			return nil
		}
	}
	return fmt.Errorf("job not found: %s", jobID)
}

func (r *InMemoryJobRepository) ReleaseFromWorker(_ context.Context, jobID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, job := range r.jobs {
		if job.job.JobID == jobID {
			r.jobs[i].job.AssignedWorkerID = ""
			r.jobs[i].job.AssignedAt = nil
			logs.Debugf("jobs-memory", "release job from worker job_id=%s deployment_id=%s status=%s",
				jobID, job.job.DeploymentID, job.status)
			return nil
		}
	}
	return fmt.Errorf("job not found: %s", jobID)
}

func (r *InMemoryJobRepository) ClaimForWorker(_ context.Context, workerID string) (domain.DeploymentJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, job := range r.jobs {
		if job.status == "queued" && job.job.AssignedWorkerID == workerID {
			r.jobs[i].status = "running"
			now := time.Now()
			r.jobs[i].job.AssignedAt = &now
			logs.Debugf("jobs-memory", "claim job for worker job_id=%s deployment_id=%s worker_id=%s queue_size=%d",
				job.job.JobID, job.job.DeploymentID, workerID, len(r.jobs))
			return r.jobs[i].job, nil
		}
	}
	return domain.DeploymentJob{}, ErrNoQueuedJobs
}

func (r *InMemoryJobRepository) GetNextUnassignedJob(_ context.Context) (domain.DeploymentJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, job := range r.jobs {
		if job.status == "queued" && job.job.AssignedWorkerID == "" {
			logs.Debugf("jobs-memory", "found unassigned job job_id=%s deployment_id=%s requested_by=%s",
				job.job.JobID, job.job.DeploymentID, job.job.RequestedBy)
			return job.job, nil
		}
	}
	return domain.DeploymentJob{}, ErrNoQueuedJobs
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
	res, err := r.db.ExecContext(ctx, q, jobID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("job not found: %s", jobID)
	}
	return err
}

func (r *PostgresJobRepository) MarkFailed(ctx context.Context, jobID string, errText string) error {
	const q = `UPDATE deployment_jobs SET status='failed', finished_at=NOW(), last_error=$2 WHERE job_id=$1`
	logs.Debugf("jobs-postgres", "mark failed job_id=%s", jobID)
	res, err := r.db.ExecContext(ctx, q, jobID, errText)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("job not found: %s", jobID)
	}
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
	res, err := r.db.ExecContext(ctx, q, workerID, jobID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("job not assignable (missing or not queued): %s", jobID)
	}
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
	res, err := r.db.ExecContext(ctx, q, jobID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("job not found: %s", jobID)
	}
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
