package telemetry

import (
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	registerOnce sync.Once

	// Control-plane metrics
	DeployRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "control_plane_deploy_requests_total",
			Help: "Total deploy requests accepted by control-plane.",
		},
		[]string{"scaling_mode"},
	)

	WorkerJobsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "control_plane_worker_jobs_total",
			Help: "Total deployment jobs processed by worker and grouped by final status.",
		},
		[]string{"status"},
	)

	WorkerJobDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "control_plane_worker_job_duration_seconds",
			Help:    "End-to-end worker job execution duration.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"status"},
	)

	// Deployment-level metrics
	DeploymentRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "deployment_requests_total",
			Help: "Total HTTP requests per deployment.",
		},
		[]string{"deployment_id", "status_code"},
	)

	DeploymentRequestLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "deployment_request_latency_seconds",
			Help:    "HTTP request latency per deployment in seconds.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"deployment_id"},
	)

	DeploymentBandwidth = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "deployment_bandwidth_bytes_total",
			Help: "Total bandwidth transferred per deployment.",
		},
		[]string{"deployment_id", "direction"}, // direction: sent|received
	)

	DeploymentPods = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "deployment_pods",
			Help: "Current pod count per deployment.",
		},
		[]string{"deployment_id", "type"}, // type: current|desired
	)

	DeploymentCPUUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "deployment_cpu_usage_percent",
			Help: "Current CPU usage percentage per deployment.",
		},
		[]string{"deployment_id"},
	)

	DeploymentMemoryUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "deployment_memory_usage_mb",
			Help: "Current memory usage in MB per deployment.",
		},
		[]string{"deployment_id"},
	)
)

func Register() {
	registerOnce.Do(func() {
		prometheus.MustRegister(
			DeployRequestsTotal,
			WorkerJobsTotal,
			WorkerJobDurationSeconds,
			DeploymentRequestsTotal,
			DeploymentRequestLatency,
			DeploymentBandwidth,
			DeploymentPods,
			DeploymentCPUUsage,
			DeploymentMemoryUsage,
		)
	})
}

func ObserveDeployRequest(scalingMode string) {
	if scalingMode == "" {
		scalingMode = "none"
	}
	DeployRequestsTotal.WithLabelValues(scalingMode).Inc()
}

func ObserveWorkerJob(status string, startedAt time.Time) {
	if status == "" {
		status = "unknown"
	}
	WorkerJobsTotal.WithLabelValues(status).Inc()
	if !startedAt.IsZero() {
		WorkerJobDurationSeconds.WithLabelValues(status).Observe(time.Since(startedAt).Seconds())
	}
}

// ObserveDeploymentRequest records a single HTTP request to a deployment
func ObserveDeploymentRequest(deploymentID string, statusCode int, latencySeconds float64, bytesSent, bytesReceived int64) {
	DeploymentRequestsTotal.WithLabelValues(deploymentID, fmt.Sprintf("%d", statusCode)).Inc()
	DeploymentRequestLatency.WithLabelValues(deploymentID).Observe(latencySeconds)

	if bytesSent > 0 {
		DeploymentBandwidth.WithLabelValues(deploymentID, "sent").Add(float64(bytesSent))
	}
	if bytesReceived > 0 {
		DeploymentBandwidth.WithLabelValues(deploymentID, "received").Add(float64(bytesReceived))
	}
}

// UpdateDeploymentPods updates the current and desired pod counts
func UpdateDeploymentPods(deploymentID string, current, desired int) {
	DeploymentPods.WithLabelValues(deploymentID, "current").Set(float64(current))
	DeploymentPods.WithLabelValues(deploymentID, "desired").Set(float64(desired))
}

// UpdateDeploymentResourceUsage updates CPU and memory usage metrics
func UpdateDeploymentResourceUsage(deploymentID string, cpuPercent, memoryMB float64) {
	DeploymentCPUUsage.WithLabelValues(deploymentID).Set(cpuPercent)
	DeploymentMemoryUsage.WithLabelValues(deploymentID).Set(memoryMB)
}
