package domain

import "time"

// DeploymentMetrics holds aggregated metrics for a deployment
type DeploymentMetrics struct {
	DeploymentID       string    `json:"deployment_id"`
	RequestCountTotal  int64     `json:"request_count_total"`
	RequestCount1h     int64     `json:"request_count_1h"`
	RequestCount24h    int64     `json:"request_count_24h"`
	RequestsPerSecond  float64   `json:"requests_per_second"`
	BandwidthSentBytes int64     `json:"bandwidth_sent_bytes"`
	BandwidthRecvBytes int64     `json:"bandwidth_received_bytes"`
	LatencyP50Ms       float64   `json:"latency_p50_ms"`
	LatencyP90Ms       float64   `json:"latency_p90_ms"`
	LatencyP99Ms       float64   `json:"latency_p99_ms"`
	CurrentPods        int       `json:"current_pods"`
	DesiredPods        int       `json:"desired_pods"`
	CPUUsagePercent    float64   `json:"cpu_usage_percent"`
	MemoryUsageMB      float64   `json:"memory_usage_mb"`
	LastUpdated        time.Time `json:"last_updated"`
}

// DeploymentRequest represents a single HTTP request to a deployment
type DeploymentRequest struct {
	ID            int64     `json:"id"`
	DeploymentID  string    `json:"deployment_id"`
	Timestamp     time.Time `json:"timestamp"`
	StatusCode    int       `json:"status_code"`
	LatencyMs     float64   `json:"latency_ms"`
	BytesSent     int64     `json:"bytes_sent"`
	BytesReceived int64     `json:"bytes_received"`
	Path          string    `json:"path"`
}

// PodMetrics represents metrics for a single pod
type PodMetrics struct {
	PodName       string    `json:"pod_name"`
	Status        string    `json:"status"` // Running, Pending, Failed, etc.
	Ready         bool      `json:"ready"`
	Restarts      int       `json:"restarts"`
	CPUUsageMilli int64     `json:"cpu_usage_milli"` // Current CPU usage in millicores
	MemoryUsageMB float64   `json:"memory_usage_mb"` // Current memory usage in MB
	Age           string    `json:"age"`             // e.g., "2h30m"
	CreatedAt     time.Time `json:"created_at"`
}

// ResourceAllocation shows requested, limit, and current usage
type ResourceAllocation struct {
	CPURequested       int     `json:"cpu_requested_milli"`
	CPULimit           int     `json:"cpu_limit_milli"`
	CPUUsageMilli      int64   `json:"cpu_usage_milli"`
	CPUUsagePercent    float64 `json:"cpu_usage_percent"`    // vs requested
	MemoryRequested    int     `json:"memory_requested_mb"`
	MemoryLimit        int     `json:"memory_limit_mb"`
	MemoryUsageMB      float64 `json:"memory_usage_mb"`
	MemoryUsagePercent float64 `json:"memory_usage_percent"` // vs requested
}

// ScalingInfo provides scaling configuration and status
type ScalingInfo struct {
	Mode        string `json:"mode"` // "none", "horizontal"
	CurrentPods int    `json:"current_pods"`
	DesiredPods int    `json:"desired_pods"`
	MinReplicas int    `json:"min_replicas"`
	MaxReplicas int    `json:"max_replicas"`
	CPUTarget   int    `json:"cpu_target_utilization"`
	HPAEnabled  bool   `json:"hpa_enabled"`
}

// DeploymentDetails combines all deployment information
type DeploymentDetails struct {
	Deployment DeploymentRecord   `json:"deployment"`
	Metrics    DeploymentMetrics  `json:"metrics"`
	Pods       []PodMetrics       `json:"pods"`
	Resources  ResourceAllocation `json:"resources"`
	Scaling    ScalingInfo        `json:"scaling"`
}

// DeploymentSummary for list endpoint with key metrics
type DeploymentSummary struct {
	DeploymentID    string    `json:"deployment_id"`
	Subdomain       string    `json:"subdomain"`
	URL             string    `json:"url"`
	Status          string    `json:"status"`
	Package         string    `json:"package"`
	CurrentPods     int       `json:"current_pods"`
	RequestCount24h int64     `json:"request_count_24h"`
	CPUUsagePercent float64   `json:"cpu_usage_percent"`
	MemoryUsageMB   float64   `json:"memory_usage_mb"`
	LastUpdated     time.Time `json:"last_updated"`
	StartedAt       time.Time `json:"started_at"`
}
