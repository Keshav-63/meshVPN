package domain

import "time"

// Worker represents a deployment worker agent
type Worker struct {
	WorkerID          string             `json:"worker_id"`
	Name              string             `json:"name"`
	TailscaleIP       string             `json:"tailscale_ip"`
	Hostname          string             `json:"hostname"`
	Status            string             `json:"status"` // idle, busy, offline
	Capabilities      WorkerCapabilities `json:"capabilities"`
	MaxConcurrentJobs int                `json:"max_concurrent_jobs"`
	CurrentJobs       int                `json:"current_jobs"`
	LastHeartbeat     *time.Time         `json:"last_heartbeat,omitempty"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
}

// WorkerCapabilities defines what a worker can handle
type WorkerCapabilities struct {
	Runtime           string   `json:"runtime"`                     // kubernetes, docker
	K8sVersion        string   `json:"k8s_version,omitempty"`       // e.g., "v1.31"
	MemoryGB          int      `json:"memory_gb"`                   // Total memory available
	CPUCores          int      `json:"cpu_cores"`                   // Total CPU cores
	MaxConcurrentJobs int      `json:"max_concurrent_jobs"`         // How many jobs can run concurrently
	SupportedPackages []string `json:"supported_packages"`          // ["small", "medium", "large"]
}

// WorkerStatus represents the current state of a worker
type WorkerStatus string

const (
	WorkerStatusIdle    WorkerStatus = "idle"    // Ready to accept jobs
	WorkerStatusBusy    WorkerStatus = "busy"    // Currently processing jobs
	WorkerStatusOffline WorkerStatus = "offline" // Not responding to heartbeats
)
