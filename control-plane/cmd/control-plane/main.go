package main

import (
	"context"
	"log"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/analytics"
	"MeshVPN-slef-hosting/control-plane/internal/auth"
	"MeshVPN-slef-hosting/control-plane/internal/config"
	"MeshVPN-slef-hosting/control-plane/internal/httpapi"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/runtime"
	"MeshVPN-slef-hosting/control-plane/internal/service"
	"MeshVPN-slef-hosting/control-plane/internal/store"
	"MeshVPN-slef-hosting/control-plane/internal/telemetry"

	_ "MeshVPN-slef-hosting/control-plane/docs" // Import generated swagger docs
)

// @title           MeshVPN Control Plane API
// @version         1.0
// @description     API for deploying and managing applications on MeshVPN platform
// @description     This API provides endpoints for deploying applications, managing deployments, viewing logs, and monitoring analytics.

// @contact.name   API Support
// @contact.url    https://github.com/keshavstack/MeshVPN-slef-hosting

// @license.name  MIT
// @license.url   https://opensource.org/licenses/MIT

// @host      localhost:8080
// @BasePath  /

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Enter your JWT token in the format: Bearer {token}

func main() {
	cfg := config.Load()
	logs.Infof("main", "startup config runtime_backend=%s require_auth=%t multi_worker=%t control_plane_as_worker=%t db_configured=%t k8s_namespace=%s worker_interval=%s analytics_hpa=%t placement=%s",
		cfg.RuntimeBackend, cfg.RequireAuth, cfg.EnableMultiWorker, cfg.ControlPlaneAsWorker, cfg.DatabaseURL != "", cfg.K8sNamespace, cfg.WorkerPollInterval, cfg.EnableCPUHPA, cfg.JobPlacementStrategy)
	deps, cleanup, err := store.Initialize(cfg)
	if err != nil {
		// If DATABASE_URL is set but connection failed, fail hard
		if cfg.DatabaseURL != "" {
			log.Fatalf("CRITICAL: Database connection failed with DATABASE_URL set. Fix the connection string and try again. Error: %v", err)
		}

		// Otherwise, gracefully fallback to in-memory store
		log.Printf("WARNING: DATABASE_URL not set, falling back to in-memory store. Multi-worker features disabled. Error: %v", err)
		deps = store.Dependencies{
			DeploymentRepo: store.NewInMemoryDeploymentRepository(),
			JobRepo:        store.NewInMemoryJobRepository(),
			HasDatabase:    false,
		}
	}
	if cleanup != nil {
		defer cleanup()
	}

	telemetry.Register()
	logs.Infof("main", "telemetry initialized")

	driver := runtime.NewDriverFromBackend(cfg.RuntimeBackend, cfg.K8sNamespace)
	runner := runtime.NewRunnerWithDriver(driver)
	deploymentService := service.NewDeploymentService(deps.DeploymentRepo, deps.JobRepo, runner)
	logs.Infof("main", "runtime driver ready backend=%s namespace=%s", cfg.RuntimeBackend, cfg.K8sNamespace)

	workerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start either multi-worker distributor or single embedded worker
	if cfg.EnableMultiWorker && deps.WorkerRepo != nil {
		// Multi-worker mode: start job distributor (handles control-plane worker registration)
		distributor := service.NewJobDistributor(deps.JobRepo, deps.WorkerRepo, deps.DeploymentRepo, cfg)
		go distributor.Start(workerCtx)
		logs.Infof("main", "multi-worker mode enabled strategy=%s control_plane_as_worker=%t",
			cfg.JobPlacementStrategy, cfg.ControlPlaneAsWorker)

		// If control-plane acts as worker, start embedded worker to execute jobs
		if cfg.ControlPlaneAsWorker {
			worker := service.NewDeploymentWorker(deps.DeploymentRepo, deps.JobRepo, deps.WorkerRepo, runner, cfg.WorkerPollInterval, cfg.EnableCPUHPA, cfg.ControlPlaneWorkerID, true)
			go worker.Start(workerCtx)
			logs.Infof("main", "embedded worker started for control-plane (worker_id=%s)", cfg.ControlPlaneWorkerID)
		}
	} else {
		// Single-worker mode: start embedded worker
		worker := service.NewDeploymentWorker(deps.DeploymentRepo, deps.JobRepo, nil, runner, cfg.WorkerPollInterval, cfg.EnableCPUHPA, cfg.ControlPlaneWorkerID, false)
		go worker.Start(workerCtx)
		logs.Infof("main", "single-worker mode: embedded worker started")
	}

	// Start analytics collector if database is available
	if deps.HasDatabase && deps.AnalyticsRepo != nil {
		collector := analytics.NewMetricsCollector(deps.AnalyticsRepo, deps.WorkerRepo, deps.DeploymentRepo, cfg.K8sNamespace, "kubectl")
		go collector.Start(workerCtx, 1*time.Minute)
		logs.Infof("main", "analytics collector started interval=1m")
	}

	// Prepare user repository for auth middleware
	var userRepo auth.UserRepository
	if deps.UserRepo != nil {
		userRepo = deps.UserRepo
	}

	// Prepare analytics repository for analytics endpoints
	var analyticsRepo httpapi.AnalyticsRepository
	if deps.AnalyticsRepo != nil {
		analyticsRepo = deps.AnalyticsRepo
	}

	// Initialize Kubernetes client and deployment details service
	var detailsService *service.DeploymentDetailsService
	if deps.HasDatabase && deps.AnalyticsRepo != nil {
		k8sClient := analytics.NewKubernetesClient(cfg.K8sNamespace, "kubectl")
		detailsService = service.NewDeploymentDetailsService(deps.DeploymentRepo, deps.AnalyticsRepo, k8sClient)
		logs.Infof("main", "deployment details service initialized with k8s client")
	}

	logs.Infof("main", "starting router require_auth=%t has_database=%t analytics=%t details_service=%t",
		cfg.RequireAuth, deps.HasDatabase, analyticsRepo != nil, detailsService != nil)
	router := httpapi.NewRouter(cfg, deploymentService, detailsService, userRepo, analyticsRepo, deps.WorkerRepo, deps.JobRepo, deps.DeploymentRepo)
	logs.Infof("main", "router initialized")

	if err := router.Run("0.0.0.0:8080"); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}
