package config

import (
	"os"
	"strconv"
	"time"
)

type ControlPlaneConfig struct {
	DatabaseURL        string
	SupabaseJWTSecret  string
	RequireAuth        bool
	WorkerPollInterval time.Duration
	WorkerBatchSize    int
	RuntimeBackend     string
	EnableCPUHPA       bool
	K8sNamespace       string
	K8sConfigPath      string
	FrontendURL        string

	// Multi-worker configuration
	EnableMultiWorker      bool
	ControlPlaneAsWorker   bool
	ControlPlaneWorkerID   string
	ControlPlaneMaxJobs    int
	JobPlacementStrategy   string
	WorkerSharedSecret     string
	WorkerHeartbeatTimeout time.Duration
}

func Load() ControlPlaneConfig {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("SUPABASE_DB_URL")
	}

	requireAuth := true
	if raw := os.Getenv("REQUIRE_AUTH"); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			requireAuth = parsed
		}
	}

	pollInterval := 2 * time.Second
	if raw := os.Getenv("WORKER_POLL_INTERVAL"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			pollInterval = d
		}
	}

	batchSize := 3
	if raw := os.Getenv("WORKER_BATCH_SIZE"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			batchSize = parsed
		}
	}

	runtimeBackend := os.Getenv("RUNTIME_BACKEND")
	if runtimeBackend == "" {
		runtimeBackend = "docker"
	}

	enableCPUHPA := false
	if raw := os.Getenv("ENABLE_CPU_HPA"); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			enableCPUHPA = parsed
		}
	}

	k8sNamespace := os.Getenv("K8S_NAMESPACE")
	if k8sNamespace == "" {
		k8sNamespace = "default"
	}

	// Multi-worker configuration
	enableMultiWorker := false
	if raw := os.Getenv("ENABLE_MULTI_WORKER"); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			enableMultiWorker = parsed
		}
	}

	controlPlaneAsWorker := false
	if raw := os.Getenv("CONTROL_PLANE_AS_WORKER"); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			controlPlaneAsWorker = parsed
		}
	}

	controlPlaneWorkerID := os.Getenv("CONTROL_PLANE_WORKER_ID")
	if controlPlaneWorkerID == "" {
		controlPlaneWorkerID = "control-plane-local"
	}

	controlPlaneMaxJobs := 2
	if raw := os.Getenv("CONTROL_PLANE_MAX_JOBS"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			controlPlaneMaxJobs = parsed
		}
	}

	jobPlacementStrategy := os.Getenv("JOB_PLACEMENT_STRATEGY")
	if jobPlacementStrategy == "" {
		jobPlacementStrategy = "smart"
	}

	workerHeartbeatTimeout := 90 * time.Second
	if raw := os.Getenv("WORKER_HEARTBEAT_TIMEOUT"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			workerHeartbeatTimeout = d
		}
	}

	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	return ControlPlaneConfig{
		DatabaseURL:        dbURL,
		SupabaseJWTSecret:  os.Getenv("SUPABASE_JWT_SECRET"),
		RequireAuth:        requireAuth,
		WorkerPollInterval: pollInterval,
		WorkerBatchSize:    batchSize,
		RuntimeBackend:     runtimeBackend,
		EnableCPUHPA:       enableCPUHPA,
		K8sNamespace:       k8sNamespace,
		K8sConfigPath:      os.Getenv("K8S_CONFIG_PATH"),
		FrontendURL:        frontendURL,

		// Multi-worker fields
		EnableMultiWorker:      enableMultiWorker,
		ControlPlaneAsWorker:   controlPlaneAsWorker,
		ControlPlaneWorkerID:   controlPlaneWorkerID,
		ControlPlaneMaxJobs:    controlPlaneMaxJobs,
		JobPlacementStrategy:   jobPlacementStrategy,
		WorkerSharedSecret:     os.Getenv("WORKER_SHARED_SECRET"),
		WorkerHeartbeatTimeout: workerHeartbeatTimeout,
	}
}
