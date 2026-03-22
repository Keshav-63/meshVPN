package domain

import "time"

type DeploymentRecord struct {
	DeploymentID string            `json:"deployment_id"`
	RequestedBy  string            `json:"requested_by,omitempty"`
	UserID       string            `json:"user_id,omitempty"`
	Package      string            `json:"package,omitempty"`
	Repo         string            `json:"repo"`
	Subdomain    string            `json:"subdomain"`
	Port         int               `json:"port"`
	ScalingMode  string            `json:"scaling_mode,omitempty"`
	MinReplicas  int               `json:"min_replicas,omitempty"`
	MaxReplicas  int               `json:"max_replicas,omitempty"`
	CPUTarget    int               `json:"cpu_target_utilization,omitempty"`
	CPURequest   int               `json:"cpu_request_milli,omitempty"`
	CPULimit     int               `json:"cpu_limit_milli,omitempty"`
	NodeSelector map[string]string `json:"node_selector,omitempty"`
	CPUCores     float64           `json:"cpu_cores,omitempty"`
	MemoryMB     int               `json:"memory_mb,omitempty"`
	Container    string            `json:"container,omitempty"`
	Image        string            `json:"image,omitempty"`
	URL          string            `json:"url,omitempty"`
	Status       string            `json:"status"`
	Error        string            `json:"error,omitempty"`
	BuildLogs    string            `json:"build_logs,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	BuildArgs    map[string]string `json:"build_args,omitempty"`
	StartedAt    time.Time         `json:"started_at"`
	FinishedAt   *time.Time        `json:"finished_at,omitempty"`
}

func CloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	copyMap := make(map[string]string, len(values))
	for k, v := range values {
		copyMap[k] = v
	}

	return copyMap
}
