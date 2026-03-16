package domain

import "time"

type DeploymentJob struct {
	JobID        string            `json:"job_id"`
	DeploymentID string            `json:"deployment_id"`
	Repo         string            `json:"repo"`
	Subdomain    string            `json:"subdomain"`
	Port         int               `json:"port"`
	Env          map[string]string `json:"env"`
	BuildArgs    map[string]string `json:"build_args"`
	CPUCores     float64           `json:"cpu_cores"`
	MemoryMB     int               `json:"memory_mb"`
	RequestedBy  string            `json:"requested_by"`
	QueuedAt     time.Time         `json:"queued_at"`
}
