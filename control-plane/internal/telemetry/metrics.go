package telemetry

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	registerOnce sync.Once

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
)

func Register() {
	registerOnce.Do(func() {
		prometheus.MustRegister(DeployRequestsTotal, WorkerJobsTotal, WorkerJobDurationSeconds)
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
