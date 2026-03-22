package domain

import "time"

type DeploymentJob struct {
	JobID            string            `json:"job_id"`
	DeploymentID     string            `json:"deployment_id"`
	Repo             string            `json:"repo"`
	Subdomain        string            `json:"subdomain"`
	Port             int               `json:"port"`
	ScalingMode      string            `json:"scaling_mode,omitempty"`
	MinReplicas      int               `json:"min_replicas,omitempty"`
	MaxReplicas      int               `json:"max_replicas,omitempty"`
	CPUTarget        int               `json:"cpu_target_utilization,omitempty"`
	CPURequest       int               `json:"cpu_request_milli,omitempty"`
	CPULimit         int               `json:"cpu_limit_milli,omitempty"`
	NodeSelector     map[string]string `json:"node_selector,omitempty"`
	Env              map[string]string `json:"env"`
	BuildArgs        map[string]string `json:"build_args"`
	CPUCores         float64           `json:"cpu_cores"`
	MemoryMB         int               `json:"memory_mb"`
	RequestedBy      string            `json:"requested_by"`
	QueuedAt         time.Time         `json:"queued_at"`

	// Worker assignment fields (multi-worker support)
	AssignedWorkerID string     `json:"assigned_worker_id,omitempty"`
	AssignedAt       *time.Time `json:"assigned_at,omitempty"`
}
