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
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 15, 20, 30},
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

	// Platform-level metrics
	PlatformWorkers = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "platform_workers_total",
			Help: "Total number of workers by status.",
		},
		[]string{"status"}, // idle, busy, offline
	)

	PlatformWorkerCapacity = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "platform_worker_capacity",
			Help: "Worker capacity (total, used, available).",
		},
		[]string{"type"}, // total, used, available
	)

	PlatformDeployments = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "platform_deployments_total",
			Help: "Total number of deployments by status.",
		},
		[]string{"status"}, // running, failed, queued
	)

	PlatformPodsTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "platform_pods_total",
			Help: "Total number of pods across all deployments.",
		},
	)

	PlatformRequestsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "platform_requests_total",
			Help: "Total number of requests across all deployments.",
		},
	)

	PlatformBandwidthTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "platform_bandwidth_bytes_total",
			Help: "Total bandwidth across all deployments.",
		},
		[]string{"direction"}, // sent, received
	)

	// Per-worker metrics
	WorkerPods = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "worker_pods_total",
			Help: "Total pods running on each worker's cluster.",
		},
		[]string{"worker_id", "worker_name"},
	)

	WorkerCurrentJobs = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "worker_current_jobs",
			Help: "Current active jobs per worker.",
		},
		[]string{"worker_id", "worker_name"},
	)

	WorkerCPUCores = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "worker_cpu_cores",
			Help: "CPU cores available per worker.",
		},
		[]string{"worker_id", "worker_name"},
	)

	WorkerMemoryGB = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "worker_memory_gb",
			Help: "Memory in GB available per worker.",
		},
		[]string{"worker_id", "worker_name"},
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
			// Platform-level metrics
			PlatformWorkers,
			PlatformWorkerCapacity,
			PlatformDeployments,
			PlatformPodsTotal,
			PlatformRequestsTotal,
			PlatformBandwidthTotal,
			// Per-worker metrics
			WorkerPods,
			WorkerCurrentJobs,
			WorkerCPUCores,
			WorkerMemoryGB,
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
	PlatformRequestsTotal.Inc()
	DeploymentRequestLatency.WithLabelValues(deploymentID).Observe(latencySeconds)

	if bytesSent > 0 {
		DeploymentBandwidth.WithLabelValues(deploymentID, "sent").Add(float64(bytesSent))
		PlatformBandwidthTotal.WithLabelValues("sent").Add(float64(bytesSent))
	}
	if bytesReceived > 0 {
		DeploymentBandwidth.WithLabelValues(deploymentID, "received").Add(float64(bytesReceived))
		PlatformBandwidthTotal.WithLabelValues("received").Add(float64(bytesReceived))
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

// Platform-level metric update functions

// UpdatePlatformWorkers updates worker counts by status
func UpdatePlatformWorkers(idle, busy, offline int) {
	PlatformWorkers.WithLabelValues("idle").Set(float64(idle))
	PlatformWorkers.WithLabelValues("busy").Set(float64(busy))
	PlatformWorkers.WithLabelValues("offline").Set(float64(offline))
}

// UpdatePlatformWorkerCapacity updates capacity metrics
func UpdatePlatformWorkerCapacity(total, used, available int) {
	PlatformWorkerCapacity.WithLabelValues("total").Set(float64(total))
	PlatformWorkerCapacity.WithLabelValues("used").Set(float64(used))
	PlatformWorkerCapacity.WithLabelValues("available").Set(float64(available))
}

// UpdatePlatformDeployments updates deployment counts by status
func UpdatePlatformDeployments(running, failed, queued int) {
	PlatformDeployments.WithLabelValues("running").Set(float64(running))
	PlatformDeployments.WithLabelValues("failed").Set(float64(failed))
	PlatformDeployments.WithLabelValues("queued").Set(float64(queued))
}

// UpdatePlatformPodsTotal updates total pod count
func UpdatePlatformPodsTotal(total int) {
	PlatformPodsTotal.Set(float64(total))
}

// IncrementPlatformRequests increments total request counter
func IncrementPlatformRequests(count int64) {
	PlatformRequestsTotal.Add(float64(count))
}

// IncrementPlatformBandwidth increments bandwidth counters
func IncrementPlatformBandwidth(sent, received int64) {
	if sent > 0 {
		PlatformBandwidthTotal.WithLabelValues("sent").Add(float64(sent))
	}
	if received > 0 {
		PlatformBandwidthTotal.WithLabelValues("received").Add(float64(received))
	}
}

// UpdateWorkerMetrics updates per-worker metrics
func UpdateWorkerMetrics(workerID, workerName string, pods, currentJobs int, cpuCores int, memoryGB int) {
	WorkerPods.WithLabelValues(workerID, workerName).Set(float64(pods))
	WorkerCurrentJobs.WithLabelValues(workerID, workerName).Set(float64(currentJobs))
	WorkerCPUCores.WithLabelValues(workerID, workerName).Set(float64(cpuCores))
	WorkerMemoryGB.WithLabelValues(workerID, workerName).Set(float64(memoryGB))
}
