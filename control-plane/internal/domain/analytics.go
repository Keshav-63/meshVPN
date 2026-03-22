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
