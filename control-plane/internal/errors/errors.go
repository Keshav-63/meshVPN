package errors

import (
	"errors"
	"fmt"
)

// Central error definitions for MeshVPN control-plane

// ErrNotFound indicates a resource was not found
var ErrNotFound = errors.New("resource not found")

// ErrInvalidConfig indicates a configuration error
var ErrInvalidConfig = errors.New("invalid configuration")

// ErrDatabaseUnavailable indicates database connection failed
var ErrDatabaseUnavailable = errors.New("database unavailable")

// ErrNoQueuedJobs indicates no jobs are available in the queue
var ErrNoQueuedJobs = errors.New("no queued jobs")

// ErrNoAvailableWorkers indicates no workers are available
var ErrNoAvailableWorkers = errors.New("no available workers")

// ErrJobNotFound indicates a job was not found
func ErrJobNotFound(jobID string) error {
	return fmt.Errorf("job not found: %s", jobID)
}

// ErrDeploymentNotFound indicates a deployment was not found
func ErrDeploymentNotFound(deploymentID string) error {
	return fmt.Errorf("deployment not found: %s", deploymentID)
}

// ErrWorkerNotFound indicates a worker was not found
func ErrWorkerNotFound(workerID string) error {
	return fmt.Errorf("worker not found: %s", workerID)
}

// ErrInvalidInput indicates input validation failed
func ErrInvalidInput(field, reason string) error {
	return fmt.Errorf("invalid input for %s: %s", field, reason)
}

// ErrInternal indicates an internal server error
func ErrInternal(operation string, err error) error {
	return fmt.Errorf("internal error during %s: %w", operation, err)
}

// ErrMultiWorkerUnavailable indicates multi-worker mode is not available
var ErrMultiWorkerUnavailable = errors.New("multi-worker features require DATABASE_URL configuration")
