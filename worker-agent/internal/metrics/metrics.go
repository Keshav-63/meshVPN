package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	registerOnce sync.Once

	// Worker-agent specific metrics
	JobsProcessedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "worker_agent_jobs_processed_total",
			Help: "Total jobs processed by this worker agent",
		},
		[]string{"status"}, // success, failed
	)

	JobDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "worker_agent_job_duration_seconds",
			Help:    "Job execution duration in seconds",
			Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600},
		},
		[]string{"status"},
	)

	CurrentActiveJobs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "worker_agent_active_jobs",
			Help: "Number of currently active jobs on this worker",
		},
	)

	PodsManaged = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "worker_agent_pods_managed",
			Help: "Total number of pods managed by this worker",
		},
	)

	HeartbeatsSentTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "worker_agent_heartbeats_sent_total",
			Help: "Total heartbeats sent to control-plane",
		},
	)

	HeartbeatFailuresTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "worker_agent_heartbeat_failures_total",
			Help: "Total failed heartbeats to control-plane",
		},
	)

	// System resource metrics (from the worker's perspective)
	SystemCPUCores = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "worker_agent_system_cpu_cores",
			Help: "Total CPU cores available on this worker system",
		},
	)

	SystemMemoryGB = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "worker_agent_system_memory_gb",
			Help: "Total memory in GB available on this worker system",
		},
	)
)

// Register registers all worker-agent metrics
func Register() {
	registerOnce.Do(func() {
		prometheus.MustRegister(
			JobsProcessedTotal,
			JobDurationSeconds,
			CurrentActiveJobs,
			PodsManaged,
			HeartbeatsSentTotal,
			HeartbeatFailuresTotal,
			SystemCPUCores,
			SystemMemoryGB,
		)
	})
}

// RecordJobCompletion records job completion with status
func RecordJobCompletion(status string, durationSeconds float64) {
	JobsProcessedTotal.WithLabelValues(status).Inc()
	JobDurationSeconds.WithLabelValues(status).Observe(durationSeconds)
}

// SetActiveJobs sets the current number of active jobs
func SetActiveJobs(count int) {
	CurrentActiveJobs.Set(float64(count))
}

// SetPodsManaged sets the total number of pods managed
func SetPodsManaged(count int) {
	PodsManaged.Set(float64(count))
}

// RecordHeartbeat records a heartbeat attempt
func RecordHeartbeat(success bool) {
	if success {
		HeartbeatsSentTotal.Inc()
	} else {
		HeartbeatFailuresTotal.Inc()
	}
}

// SetSystemResources sets the system resource capabilities
func SetSystemResources(cpuCores int, memoryGB int) {
	SystemCPUCores.Set(float64(cpuCores))
	SystemMemoryGB.Set(float64(memoryGB))
}
